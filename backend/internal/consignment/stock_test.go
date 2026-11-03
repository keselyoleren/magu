package consignment

import (
	"context"
	"errors"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"magu/internal/catalog"
)

type fixture struct {
	svc      *Service
	db       *pgxpool.Pool
	userID   int64
	memberID int64
	keripik  int64
	sambal   int64
	air      int64
}

func setup(t *testing.T) *fixture {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL belum di-set; jalankan lewat `make test`")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)

	f := &fixture{svc: NewService(db, nil), db: db}
	_, err = db.Exec(ctx, `TRUNCATE users, members, products, goods_receipts, stock_opnames, stock_movements,
		transactions, vendor_settlements, vendor_returns
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(ctx, `
		WITH u AS (
			INSERT INTO users (username, password_hash, full_name, role) VALUES ('admin', 'x', 'Admin', 'admin')
			RETURNING id
		), m AS (
			INSERT INTO members (code, name, commission_bps) VALUES ('M1', 'Bu Sari', 2000) RETURNING id
		), p1 AS (
			INSERT INTO products (member_id, name, price) SELECT id, 'Keripik', 15000 FROM m RETURNING id
		), p2 AS (
			INSERT INTO products (member_id, name, price) SELECT id, 'Sambal', 9000 FROM m RETURNING id
		), p3 AS (
			INSERT INTO products (name, price) VALUES ('Air Mineral', 4000) RETURNING id
		)
		SELECT u.id, m.id, p1.id, p2.id, p3.id FROM u, m, p1, p2, p3`).
		Scan(&f.userID, &f.memberID, &f.keripik, &f.sambal, &f.air)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) stock(t *testing.T, productID int64) (stock, moved int) {
	err := f.db.QueryRow(context.Background(), `
		SELECT stock, COALESCE((SELECT sum(qty_change) FROM stock_movements WHERE product_id = p.id), 0)
		FROM products p WHERE id = $1`, productID).Scan(&stock, &moved)
	if err != nil {
		t.Fatal(err)
	}
	return stock, moved
}

func TestReceipt(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	id, err := f.svc.CreateReceipt(ctx, f.userID, &f.memberID, "titipan pertama",
		[]ReceiptLine{{ProductID: f.keripik, Qty: 20}, {ProductID: f.sambal, Qty: 5}})
	if err != nil {
		t.Fatal(err)
	}
	if stock, moved := f.stock(t, f.keripik); stock != 20 || moved != 20 {
		t.Errorf("keripik stock = %d, moved = %d; want 20, 20", stock, moved)
	}

	receipt, err := f.svc.GetReceipt(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.ItemCount != 2 || receipt.TotalQty != 25 || len(receipt.Items) != 2 || receipt.MemberName != "Bu Sari" {
		t.Errorf("receipt = %+v", receipt)
	}

	_, err = f.svc.CreateReceipt(ctx, f.userID, &f.memberID, "", []ReceiptLine{{ProductID: f.air, Qty: 1}})
	if !errors.Is(err, ErrWrongOwner) {
		t.Errorf("barang toko diterima atas nama member: err = %v", err)
	}
	if _, err = f.svc.CreateReceipt(ctx, f.userID, nil, "", []ReceiptLine{{ProductID: f.air, Qty: 12}}); err != nil {
		t.Fatal(err)
	}
	if stock, moved := f.stock(t, f.air); stock != 12 || moved != 12 {
		t.Errorf("air stock = %d, moved = %d; want 12, 12", stock, moved)
	}
}

func TestCancelReceipt(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	id, err := f.svc.CreateReceipt(ctx, f.userID, &f.memberID, "salah ketik",
		[]ReceiptLine{{ProductID: f.keripik, Qty: 20}, {ProductID: f.sambal, Qty: 5}})
	if err != nil {
		t.Fatal(err)
	}
	// Sebagian sudah terjual sebelum salah inputnya ketahuan: stok boleh minus.
	if _, err := f.db.Exec(ctx, `
		WITH s AS (UPDATE products SET stock = stock - 7 WHERE id = $1)
		INSERT INTO stock_movements (product_id, qty_change, reason) VALUES ($1, -7, 'sale')`, f.sambal); err != nil {
		t.Fatal(err)
	}

	if err := f.svc.CancelReceipt(ctx, f.userID, id); err != nil {
		t.Fatal(err)
	}
	if stock, moved := f.stock(t, f.keripik); stock != 0 || moved != 0 {
		t.Errorf("keripik stock = %d, moved = %d; want 0, 0", stock, moved)
	}
	if stock, moved := f.stock(t, f.sambal); stock != -7 || moved != -7 {
		t.Errorf("sambal stock = %d, moved = %d; want -7, -7", stock, moved)
	}

	if err := f.svc.CancelReceipt(ctx, f.userID, id); !errors.Is(err, ErrReceiptCancelled) {
		t.Errorf("pembatalan kedua: err = %v", err)
	}
	if stock, _ := f.stock(t, f.keripik); stock != 0 {
		t.Errorf("pembatalan kedua mengubah stok menjadi %d", stock)
	}
	if err := f.svc.CancelReceipt(ctx, f.userID, 999999); !errors.Is(err, ErrReceiptNotFound) {
		t.Errorf("penerimaan tidak ada: err = %v", err)
	}

	receipt, err := f.svc.GetReceipt(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.CancelledAt == nil || receipt.CancelledBy != "Admin" {
		t.Errorf("receipt = %+v", receipt)
	}
	today := time.Now().Format(time.DateOnly)
	rows, err := f.svc.StockSummary(ctx, &f.memberID, today, today)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Received != 0 {
			t.Errorf("%s masih terhitung diterima %d", row.Name, row.Received)
		}
	}
	card, err := f.svc.StockCard(ctx, f.keripik)
	if err != nil {
		t.Fatal(err)
	}
	if m := card.Movements[0]; m.QtyChange != -20 || m.Note != "Penerimaan dibatalkan" {
		t.Errorf("mutasi = %+v", m)
	}
}

func TestListReceiptsFilter(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	first, err := f.svc.CreateReceipt(ctx, f.userID, &f.memberID, "surat jalan SJ-77", []ReceiptLine{{ProductID: f.keripik, Qty: 2}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := f.svc.CreateReceipt(ctx, f.userID, nil, "", []ReceiptLine{{ProductID: f.air, Qty: 3}})
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().Format(time.DateOnly)
	yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)

	for name, c := range map[string]struct {
		filter DocFilter
		want   []int64
	}{
		"semua":        {DocFilter{}, []int64{store, first}},
		"penitip":      {DocFilter{MemberID: f.memberID}, []int64{first}},
		"barang toko":  {DocFilter{MemberID: catalog.StoreOwned}, []int64{store}},
		"catatan":      {DocFilter{Search: "sj-77"}, []int64{first}},
		"nama barang":  {DocFilter{Search: "mineral"}, []int64{store}},
		"nomor":        {DocFilter{Search: "#" + strconv.FormatInt(first, 10)}, []int64{first}},
		"hari ini":     {DocFilter{From: today, To: today}, []int64{store, first}},
		"sebelum ini":  {DocFilter{To: yesterday}, nil},
		"gabungan":     {DocFilter{MemberID: f.memberID, From: today, Search: "keripik"}, []int64{first}},
		"tidak ketemu": {DocFilter{Search: "tidak ada"}, nil},
	} {
		got, err := f.svc.ListReceipts(ctx, c.filter)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		ids := make([]int64, 0, len(got))
		for _, r := range got {
			ids = append(ids, r.ID)
		}
		if !slices.Equal(ids, c.want) {
			t.Errorf("%s: ids = %v; want %v", name, ids, c.want)
		}
	}

	if _, err := f.svc.CreateReturn(ctx, f.userID, f.memberID, "tidak laku", []ReceiptLine{{ProductID: f.keripik, Qty: 1}}); err != nil {
		t.Fatal(err)
	}
	returns, err := f.svc.ListReturns(ctx, DocFilter{MemberID: f.memberID, From: today, To: today, Search: "keripik"})
	if err != nil || len(returns) != 1 || returns[0].CancelledAt != nil {
		t.Errorf("retur = %+v, %v", returns, err)
	}
}

func TestOpname(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	_, err := f.svc.CreateReceipt(ctx, f.userID, &f.memberID, "",
		[]ReceiptLine{{ProductID: f.keripik, Qty: 20}, {ProductID: f.sambal, Qty: 5}})
	if err != nil {
		t.Fatal(err)
	}

	id, err := f.svc.CreateOpname(ctx, f.userID, &f.memberID, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.PostOpname(ctx, id, f.userID); !errors.Is(err, ErrNothingCounted) {
		t.Fatalf("posting tanpa hitungan: err = %v", err)
	}

	physical := 17
	if err := f.svc.SetPhysicalQty(ctx, id, f.keripik, &physical); err != nil {
		t.Fatal(err)
	}
	// Stok berubah setelah draft dibuat: posting harus memakai angka terbaru (18), bukan 20.
	if _, err := f.db.Exec(ctx, `
		WITH s AS (UPDATE products SET stock = stock - 2 WHERE id = $1)
		INSERT INTO stock_movements (product_id, qty_change, reason) VALUES ($1, -2, 'sale')`, f.keripik); err != nil {
		t.Fatal(err)
	}

	if err := f.svc.PostOpname(ctx, id, f.userID); err != nil {
		t.Fatal(err)
	}
	if stock, moved := f.stock(t, f.keripik); stock != 17 || moved != 17 {
		t.Errorf("keripik stock = %d, moved = %d; want 17, 17", stock, moved)
	}
	if stock, _ := f.stock(t, f.sambal); stock != 5 {
		t.Errorf("sambal yang tidak dihitung berubah menjadi %d", stock)
	}

	opname, err := f.svc.GetOpname(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if opname.Status != "posted" || opname.ItemCount != 2 || opname.Counted != 1 || opname.DiffCount != 1 {
		t.Errorf("opname = %+v", opname)
	}
	for _, item := range opname.Items {
		if item.ProductID == f.keripik && item.SystemQty != 18 {
			t.Errorf("system_qty keripik = %d; want 18", item.SystemQty)
		}
	}

	if err := f.svc.PostOpname(ctx, id, f.userID); !errors.Is(err, ErrOpnamePosted) {
		t.Errorf("posting kedua: err = %v", err)
	}
	if err := f.svc.SetPhysicalQty(ctx, id, f.sambal, &physical); !errors.Is(err, ErrOpnamePosted) {
		t.Errorf("mengubah opname yang sudah diposting: err = %v", err)
	}
	if err := f.svc.DeleteOpname(ctx, id); !errors.Is(err, ErrOpnamePosted) {
		t.Errorf("menghapus opname yang sudah diposting: err = %v", err)
	}

	card, err := f.svc.StockCard(ctx, f.keripik)
	if err != nil {
		t.Fatal(err)
	}
	if card.Stock != 17 || len(card.Movements) != 3 || card.Movements[0].Reason != "opname" || card.Movements[0].QtyChange != -1 {
		t.Errorf("kartu stok = %+v", card)
	}
}

func TestAdjustStocks(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	qty := func(n int32) *int32 { return &n }

	if _, err := f.svc.CreateReceipt(ctx, f.userID, &f.memberID, "",
		[]ReceiptLine{{ProductID: f.keripik, Qty: 20}, {ProductID: f.sambal, Qty: 5}}); err != nil {
		t.Fatal(err)
	}
	// Dua pemilik dalam satu simpanan, salah satunya stoknya naik dari nol.
	err := f.svc.AdjustStocks(ctx, f.userID, []StockChange{
		{ID: f.keripik, Stock: qty(18)}, {ID: f.sambal, Stock: qty(7)}, {ID: f.air, Stock: qty(3)},
	}, "2 rusak")
	if err != nil {
		t.Fatal(err)
	}
	for product, want := range map[int64]int{f.keripik: 18, f.sambal: 7, f.air: 3} {
		if stock, moved := f.stock(t, product); stock != want || moved != want {
			t.Errorf("produk %d: stok = %d, mutasi = %d; want %d", product, stock, moved, want)
		}
	}

	card, err := f.svc.StockCard(ctx, f.keripik)
	if err != nil {
		t.Fatal(err)
	}
	if m := card.Movements[0]; m.Reason != "opname" || m.QtyChange != -2 || m.Note != "Koreksi stok: 2 rusak" {
		t.Errorf("mutasi = %+v", m)
	}
	opname, err := f.svc.GetOpname(ctx, *card.Movements[0].RefID)
	if err != nil {
		t.Fatal(err)
	}
	if opname.Status != "posted" || opname.ItemCount != 2 || opname.MemberID == nil || *opname.MemberID != f.memberID {
		t.Errorf("opname member = %+v", opname)
	}
	opnames, err := f.svc.ListOpnames(ctx)
	if err != nil || len(opnames) != 2 {
		t.Errorf("satu dokumen per pemilik: %d opname, err = %v", len(opnames), err)
	}

	// Jumlah yang sama tidak boleh menulis mutasi: stock_movements menolak qty_change 0.
	if err := f.svc.AdjustStocks(ctx, f.userID, []StockChange{{ID: f.keripik, Stock: qty(18)}}, ""); err != nil {
		t.Fatal(err)
	}
	if stock, moved := f.stock(t, f.keripik); stock != 18 || moved != 18 {
		t.Errorf("setelah koreksi tanpa selisih: stok = %d, mutasi = %d", stock, moved)
	}

	// Satu produk yang hilang membatalkan semuanya.
	err = f.svc.AdjustStocks(ctx, f.userID, []StockChange{{ID: f.keripik, Stock: qty(1)}, {ID: 999999, Stock: qty(1)}}, "")
	if !errors.Is(err, ErrProductNotFound) {
		t.Errorf("produk tidak ada: err = %v", err)
	}
	if stock, _ := f.stock(t, f.keripik); stock != 18 {
		t.Errorf("koreksi yang gagal tetap mengubah stok menjadi %d", stock)
	}
}

func TestVendorReturn(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	if _, err := f.svc.CreateReceipt(ctx, f.userID, &f.memberID, "", []ReceiptLine{{ProductID: f.keripik, Qty: 20}}); err != nil {
		t.Fatal(err)
	}
	id, err := f.svc.CreateReturn(ctx, f.userID, f.memberID, "tidak laku", []ReceiptLine{{ProductID: f.keripik, Qty: 6}})
	if err != nil {
		t.Fatal(err)
	}
	if stock, moved := f.stock(t, f.keripik); stock != 14 || moved != 14 {
		t.Errorf("stok = %d, mutasi = %d; want 14, 14", stock, moved)
	}
	ret, err := f.svc.GetReturn(ctx, id)
	if err != nil || ret.TotalQty != 6 || ret.Note != "tidak laku" || len(ret.Items) != 1 {
		t.Errorf("retur = %+v, %v", ret, err)
	}
	if _, err := f.svc.CreateReturn(ctx, f.userID, f.memberID, "", []ReceiptLine{{ProductID: f.air, Qty: 1}}); !errors.Is(err, ErrWrongOwner) {
		t.Errorf("retur barang toko atas nama member: err = %v", err)
	}

	today := time.Now().Format(time.DateOnly)
	rows, err := f.svc.StockSummary(ctx, &f.memberID, today, today)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ringkasan = %+v, %v", rows, err)
	}
	k := rows[0]
	if k.Name != "Keripik" || k.Received != 20 || k.Returned != 6 || k.Sold != 0 || k.Stock != 14 {
		t.Errorf("ringkasan keripik = %+v", k)
	}
}
