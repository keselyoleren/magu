package catalog

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBarcodePrefix(t *testing.T) {
	for code, want := range map[string]string{"km": "KM", "Klei-Mate 01": "KLEIMATE01", "--": "", "M1": "M1"} {
		if got := barcodePrefix(code); got != want {
			t.Errorf("barcodePrefix(%q) = %q; want %q", code, got, want)
		}
	}
}

func TestNextBarcode(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL belum di-set; jalankan lewat `make test`")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, `TRUNCATE members, products RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	var km, m1, m10 int64
	err = db.QueryRow(ctx, `
		WITH a AS (INSERT INTO members (code, name, commission_bps) VALUES ('km', 'Klei Mate', 2000) RETURNING id),
		     b AS (INSERT INTO members (code, name, commission_bps) VALUES ('M1', 'Satu', 2000) RETURNING id),
		     c AS (INSERT INTO members (code, name, commission_bps) VALUES ('M10', 'Sepuluh', 2000) RETURNING id)
		SELECT a.id, b.id, c.id FROM a, b, c`).Scan(&km, &m1, &m10)
	if err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, nil, "mg")

	create := func(memberID *int64, barcode string) string {
		p := Product{MemberID: memberID, Barcode: barcode, Name: "Uji", Price: 1000, IsActive: true}
		if err := svc.Create(ctx, &p); err != nil {
			t.Fatal(err)
		}
		return p.Barcode
	}
	steps := []struct {
		member *int64
		typed  string
		want   string
	}{
		{&km, "", "KM00001"},
		{&km, "", "KM00002"},
		{nil, "", "MG00001"},
		{&km, "899123", "899123"},
		{&km, "", "KM00003"},
		{&m10, "", "M1000001"},
		{&m1, "", "M100001"},
		{&m1, "", "M100002"},
		{&m10, "", "M1000002"},
	}
	for i, s := range steps {
		if got := create(s.member, s.typed); got != s.want {
			t.Errorf("langkah %d: barcode = %s; want %s", i+1, got, s.want)
		}
	}

	missing := int64(999)
	if _, err := svc.NextBarcode(ctx, &missing); err != ErrMemberNotFound {
		t.Errorf("member tak dikenal: err = %v", err)
	}
	n, problems, err := svc.Import(ctx, []byte("name,price,member_code,barcode\nImpor A,1000,km,\nImpor B,2000,,\nImpor C,3000,km,\nImpor D,4000,,777\n"))
	if err != nil || len(problems) != 0 || n != 4 {
		t.Fatalf("Import = %d, %v, %v", n, problems, err)
	}
	for name, want := range map[string]string{"Impor A": "KM00004", "Impor C": "KM00005", "Impor B": "MG00002", "Impor D": "777"} {
		got, _, _ := svc.List(ctx, name, 0, 0, 0)
		if len(got) != 1 || got[0].Barcode != want {
			t.Errorf("barcode %s = %+v; want %s", name, got, want)
		}
	}

	all, _, _ := svc.List(ctx, "", 0, 0, 0)
	changes := []PriceChange{{ID: all[0].ID, Price: 25000, IsActive: true}, {ID: all[1].ID, Price: all[1].Price, IsActive: all[1].IsActive}}
	if n, err := svc.SetPrices(ctx, changes); err != nil || n != 1 {
		t.Errorf("SetPrices = %d, %v; want 1 produk berubah", n, err)
	}
	if p, _ := svc.Get(ctx, all[0].ID); p.Price != 25000 {
		t.Errorf("harga setelah SetPrices = %d", p.Price)
	}

	found, _, err := svc.List(ctx, "km00002", 0, 0, 0)
	if err != nil || len(found) != 1 {
		t.Errorf("cari dengan huruf kecil: %d hasil, %v", len(found), err)
	}
}

func TestDeleteProduct(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL belum di-set; jalankan lewat `make test`")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(ctx, `TRUNCATE members, products RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	svc := NewService(db, nil, "mg")
	unused := Product{Name: "Salah input", Price: 1000, IsActive: true}
	used := Product{Name: "Sudah diterima", Price: 1000, IsActive: true}
	for _, p := range []*Product{&unused, &used} {
		if err := svc.Create(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(ctx, `INSERT INTO stock_movements (product_id, qty_change, reason) VALUES ($1, 3, 'receipt')`, used.ID); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, used.ID); err != ErrProductInUse {
		t.Errorf("hapus produk berriwayat: err = %v", err)
	}
	if err := svc.Delete(ctx, unused.ID); err != nil {
		t.Errorf("hapus produk tanpa riwayat: %v", err)
	}
	if err := svc.Delete(ctx, unused.ID); err != ErrProductNotFound {
		t.Errorf("hapus dua kali: err = %v", err)
	}
	if n, err := svc.ActiveCount(ctx); err != nil || n != 1 {
		t.Errorf("ActiveCount = %d, %v; want 1", n, err)
	}
}
