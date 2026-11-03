package consignment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"magu/internal/drive"
	"magu/internal/sales"
)

type fakeDrive struct {
	mu      sync.Mutex
	folders int
	shared  []string
	revoked []string
	uploads int
	down    bool
}

func (f *fakeDrive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.URL.Path == "/token":
		fmt.Fprint(w, `{"access_token":"abc","expires_in":3600}`)
	case f.down:
		w.WriteHeader(http.StatusServiceUnavailable)
	case r.Method == http.MethodPost && r.URL.Path == "/drive/v3/files":
		f.folders++
		fmt.Fprintf(w, `{"id":"folder%d"}`, f.folders)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/permissions"):
		var perm struct {
			Email string `json:"emailAddress"`
		}
		json.NewDecoder(r.Body).Decode(&perm)
		f.shared = append(f.shared, perm.Email)
		fmt.Fprintf(w, `{"id":"perm%d"}`, len(f.shared))
	case r.Method == http.MethodDelete:
		f.revoked = append(f.revoked, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet:
		fmt.Fprint(w, `{"files":[]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
		f.uploads++
		fmt.Fprint(w, `{"id":"file1"}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (f *fixture) withDrive(t *testing.T) *fakeDrive {
	fake := &fakeDrive{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	f.svc = NewService(f.db, drive.NewClient(drive.Config{
		ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", APIBase: srv.URL, TokenURL: srv.URL + "/token",
	}))
	return fake
}

func (f *fixture) setEmail(t *testing.T, email string) {
	_, err := f.db.Exec(context.Background(), `UPDATE members SET email = NULLIF($2, '') WHERE id = $1`, f.memberID, email)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMemberFolderFollowsEmail(t *testing.T) {
	f := setup(t)
	fake := f.withDrive(t)
	ctx := context.Background()

	folder, err := f.svc.SyncMemberFolder(ctx, f.memberID)
	if err != nil || folder != "folder1" || len(fake.shared) != 0 {
		t.Fatalf("tanpa email: folder %q, err %v, dibagikan %v", folder, err, fake.shared)
	}

	f.setEmail(t, "sari@example.com")
	f.svc.SyncMemberFolder(ctx, f.memberID)
	f.svc.SyncMemberFolder(ctx, f.memberID)
	if fake.folders != 1 || len(fake.shared) != 1 {
		t.Errorf("sinkron berulang: %d folder, dibagikan %v", fake.folders, fake.shared)
	}

	f.setEmail(t, "baru@example.com")
	if _, err := f.svc.SyncMemberFolder(ctx, f.memberID); err != nil {
		t.Fatal(err)
	}
	if len(fake.revoked) != 1 || fake.revoked[0] != "perm1" || fake.shared[1] != "baru@example.com" {
		t.Errorf("ganti email: dicabut %v, dibagikan %v", fake.revoked, fake.shared)
	}

	f.setEmail(t, "")
	if _, err := f.svc.SyncMemberFolder(ctx, f.memberID); err != nil {
		t.Fatal(err)
	}
	if len(fake.revoked) != 2 || fake.revoked[1] != "perm2" || len(fake.shared) != 2 {
		t.Errorf("hapus email: dicabut %v, dibagikan %v", fake.revoked, fake.shared)
	}

	if _, err := f.svc.SyncMemberFolder(ctx, 9999); !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("member tak dikenal: err = %v", err)
	}
}

func TestSyncMemberFolderDisabled(t *testing.T) {
	f := setup(t)
	if _, err := f.svc.SyncMemberFolder(context.Background(), f.memberID); !errors.Is(err, ErrDriveDisabled) {
		t.Errorf("err = %v", err)
	}
}

func TestDriveFailureLeavesSettlementPaid(t *testing.T) {
	f := setup(t)
	fake := f.withDrive(t)
	ctx := context.Background()

	f.sell(t, 1, sales.CartLine{ProductID: f.keripik, Qty: 1})
	id, err := f.svc.CreateSettlement(ctx, f.userID, f.memberID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.PaySettlement(ctx, id); err != nil {
		t.Fatal(err)
	}

	fake.down = true
	_, err = f.svc.SendToMemberFolder(ctx, f.memberID, "bagi-hasil.xlsx", []byte("x"))
	var failed *drive.Error
	if !errors.As(err, &failed) {
		t.Fatalf("err = %v", err)
	}
	st, err := f.svc.GetSettlement(ctx, id)
	if err != nil || st.Status != "paid" || st.DriveSentAt != nil {
		t.Errorf("setelah Drive gagal: %+v, %v", st, err)
	}

	fake.down = false
	if _, err := f.svc.SendToMemberFolder(ctx, f.memberID, "bagi-hasil.xlsx", []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.MarkSettlementSent(ctx, id); err != nil {
		t.Fatal(err)
	}
	st, _ = f.svc.GetSettlement(ctx, id)
	if fake.uploads != 1 || st.DriveSentAt == nil {
		t.Errorf("setelah kirim ulang: %d unggahan, %+v", fake.uploads, st)
	}
}

func TestDriveHandlers(t *testing.T) {
	f := setup(t)
	fake := f.withDrive(t)
	ctx := context.Background()
	app := fiber.New(fiber.Config{ErrorHandler: func(c *fiber.Ctx, err error) error {
		var fe *fiber.Error
		if errors.As(err, &fe) {
			return c.SendStatus(fe.Code)
		}
		t.Errorf("error tak terduga: %v", err)
		return c.SendStatus(fiber.StatusInternalServerError)
	}})
	NewHandler(f.svc, "Toko").Routes(app, func(c *fiber.Ctx) error { return c.Next() })
	post := func(path string) int {
		res, err := app.Test(httptest.NewRequest(http.MethodPost, path, nil), 5000)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode
	}

	f.sell(t, 1, sales.CartLine{ProductID: f.keripik, Qty: 1})
	id, err := f.svc.CreateSettlement(ctx, f.userID, f.memberID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/settlements/%d/drive", id)
	if got := post(path); got != fiber.StatusUnprocessableEntity || fake.uploads != 0 {
		t.Errorf("kirim draft: status %d, %d unggahan", got, fake.uploads)
	}
	if err := f.svc.PaySettlement(ctx, id); err != nil {
		t.Fatal(err)
	}
	fake.down = true
	if got := post(path); got != fiber.StatusBadGateway {
		t.Errorf("Drive mati: status %d", got)
	}
	fake.down = false
	if got := post(path); got != fiber.StatusOK || fake.uploads != 1 {
		t.Errorf("kirim yang dibayar: status %d, %d unggahan", got, fake.uploads)
	}

	stock := fmt.Sprintf("/reports/stock/drive?member_id=%d&from=2026-10-01&to=2026-10-31", f.memberID)
	if got := post(stock); got != fiber.StatusOK || fake.uploads != 2 {
		t.Errorf("kirim stok: status %d, %d unggahan", got, fake.uploads)
	}
	if got := post("/reports/stock/drive?member_id=0&from=2026-10-01&to=2026-10-31"); got != fiber.StatusUnprocessableEntity {
		t.Errorf("stok barang toko: status %d", got)
	}
}
