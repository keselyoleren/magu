package sales

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	svc     *Service
	db      *pgxpool.Pool
	userID  int64
	keripik int64
	sambal  int64
	air     int64
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

	f := &fixture{svc: NewService(db), db: db}
	_, err = db.Exec(ctx, `TRUNCATE users, members, products, transactions, vendor_settlements, stock_movements
		RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(ctx, `
		WITH u AS (
			INSERT INTO users (username, password_hash, full_name, role) VALUES ('kasir', 'x', 'Kasir', 'cashier')
			RETURNING id
		), m AS (
			INSERT INTO members (code, name, commission_bps) VALUES ('M1', 'Bu Sari', 2000) RETURNING id
		), p1 AS (
			INSERT INTO products (member_id, name, price, stock) SELECT id, 'Keripik', 15000, 10 FROM m RETURNING id
		), p2 AS (
			INSERT INTO products (member_id, name, price, commission_bps, stock)
			SELECT id, 'Sambal', 9999, 1500, 5 FROM m RETURNING id
		), p3 AS (
			INSERT INTO products (name, price, stock) VALUES ('Air Mineral', 4000, 50) RETURNING id
		)
		SELECT u.id, p1.id, p2.id, p3.id FROM u, p1, p2, p3`).Scan(&f.userID, &f.keripik, &f.sambal, &f.air)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) stock(t *testing.T, productID int64) (stock int) {
	if err := f.db.QueryRow(context.Background(), `SELECT stock FROM products WHERE id = $1`, productID).Scan(&stock); err != nil {
		t.Fatal(err)
	}
	return stock
}

func TestCheckoutCash(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tendered := int64(60000)
	in := CheckoutInput{
		ClientRef: "7b1f5f0e-0000-4000-8000-000000000001",
		Lines:     []CartLine{{ProductID: f.keripik, Qty: 2}, {ProductID: f.sambal, Qty: 1}, {ProductID: f.air, Qty: 1}, {ProductID: f.keripik, Qty: 1}},
		Payments:  []PaymentInput{{Method: "cash", Amount: 58999, Tendered: &tendered}},
	}

	id, err := f.svc.Checkout(ctx, f.userID, in)
	if err != nil {
		t.Fatal(err)
	}
	sale, err := f.svc.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if sale.Status != "paid" || sale.Total != 58999 || len(sale.Items) != 3 || sale.Methods != "cash" || sale.PaidAt == nil {
		t.Errorf("sale = %+v", sale)
	}
	if got := f.stock(t, f.keripik); got != 7 {
		t.Errorf("stok keripik = %d; want 7", got)
	}

	var store, vendor int64
	err = f.db.QueryRow(ctx, `
		SELECT sum(commission_amount), sum(vendor_amount) FROM transaction_items WHERE transaction_id = $1`, id).
		Scan(&store, &vendor)
	if err != nil {
		t.Fatal(err)
	}
	// keripik 45.000 @20% = 9.000; sambal 9.999 @15% = 1.500; air 4.000 milik toko = 4.000
	if store != 14500 || vendor != 44499 {
		t.Errorf("hak toko = %d, hak vendor = %d; want 14500, 44499", store, vendor)
	}

	again, err := f.svc.Checkout(ctx, f.userID, in)
	if err != nil || again != id {
		t.Errorf("checkout ulang dengan client_ref sama: id = %d, err = %v; want %d", again, err, id)
	}
	if got := f.stock(t, f.keripik); got != 7 {
		t.Errorf("stok keripik setelah checkout ulang = %d; want 7", got)
	}
}

func TestCheckoutRejects(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	lines := []CartLine{{ProductID: f.keripik, Qty: 1}}

	_, err := f.svc.Checkout(ctx, f.userID, CheckoutInput{
		ClientRef: "7b1f5f0e-0000-4000-8000-000000000002", Lines: lines,
		Payments: []PaymentInput{{Method: "cash", Amount: 14000}},
	})
	if !errors.Is(err, ErrPaymentMismatch) {
		t.Errorf("pembayaran kurang: err = %v", err)
	}

	_, err = f.svc.Checkout(ctx, f.userID, CheckoutInput{
		ClientRef: "7b1f5f0e-0000-4000-8000-000000000003", Lines: []CartLine{{ProductID: 9999, Qty: 1}},
		Payments: []PaymentInput{{Method: "cash", Amount: 1}},
	})
	if !errors.Is(err, ErrProductNotFound) {
		t.Errorf("produk tak dikenal: err = %v", err)
	}
	if got := f.stock(t, f.keripik); got != 10 {
		t.Errorf("stok berubah oleh checkout yang ditolak: %d", got)
	}
}

func TestSplitAndVoid(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tendered := int64(10000)
	ref := "APPR123"

	id, err := f.svc.Checkout(ctx, f.userID, CheckoutInput{
		ClientRef: "7b1f5f0e-0000-4000-8000-000000000004",
		Lines:     []CartLine{{ProductID: f.keripik, Qty: 1}},
		Payments: []PaymentInput{
			{Method: "debit", Amount: 10000, Reference: &ref},
			{Method: "cash", Amount: 5000, Tendered: &tendered},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sale, _ := f.svc.Get(ctx, id)
	if sale.Status != "paid" || len(sale.Payments) != 2 || sale.Methods != "cash,debit" {
		t.Errorf("sale = %+v", sale)
	}

	if _, err := f.svc.Void(ctx, id, f.userID); err != nil {
		t.Fatal(err)
	}
	if got := f.stock(t, f.keripik); got != 10 {
		t.Errorf("stok setelah void = %d; want 10", got)
	}
	var moved int
	f.db.QueryRow(ctx, `SELECT sum(qty_change) FROM stock_movements WHERE product_id = $1`, f.keripik).Scan(&moved)
	if moved != 0 {
		t.Errorf("jumlah mutasi setelah jual lalu void = %d; want 0", moved)
	}
	if _, err := f.svc.Void(ctx, id, f.userID); !errors.Is(err, ErrAlreadyVoid) {
		t.Errorf("void kedua: err = %v", err)
	}
}

func TestVoidSettled(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	id, err := f.svc.Checkout(ctx, f.userID, CheckoutInput{
		ClientRef: "7b1f5f0e-0000-4000-8000-000000000005",
		Lines:     []CartLine{{ProductID: f.keripik, Qty: 1}},
		Payments:  []PaymentInput{{Method: "cash", Amount: 15000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.db.Exec(ctx, `
		WITH s AS (
			INSERT INTO vendor_settlements (member_id, period_start, period_end, created_by)
			VALUES (1, current_date, current_date, $2) RETURNING id
		)
		UPDATE transaction_items SET settlement_id = (SELECT id FROM s) WHERE transaction_id = $1`, id, f.userID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Void(ctx, id, f.userID); !errors.Is(err, ErrSettled) {
		t.Errorf("void transaksi yang sudah dibagi hasil: err = %v", err)
	}
}

func TestQRIS(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tendered := int64(5000)

	id, err := f.svc.Checkout(ctx, f.userID, CheckoutInput{
		ClientRef: "7b1f5f0e-0000-4000-8000-000000000006",
		Lines:     []CartLine{{ProductID: f.keripik, Qty: 1}},
		Payments: []PaymentInput{
			{Method: "cash", Amount: 5000, Tendered: &tendered},
			{Method: "qris", Amount: 10000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sale, _ := f.svc.Get(ctx, id)
	if sale.Status != "pending" || f.stock(t, f.keripik) != 10 {
		t.Fatalf("sebelum QRIS masuk: status = %s, stok = %d", sale.Status, f.stock(t, f.keripik))
	}
	charges, err := f.svc.UnchargedQRIS(ctx, id)
	if err != nil || len(charges) != 1 || charges[0].Amount != 10000 {
		t.Fatalf("UnchargedQRIS = %+v, %v", charges, err)
	}
	ref := charges[0].ProviderRef

	if u, err := f.svc.ApplyGatewayResult(ctx, ref, "paid", 9000, nil); u != nil || err != nil {
		t.Errorf("nominal salah: %+v, %v", u, err)
	}
	u, err := f.svc.ApplyGatewayResult(ctx, ref, "paid", 10000, []byte(`{"transaction_status":"settlement"}`))
	if err != nil || u == nil || !u.TransactionPaid || u.TransactionID != id {
		t.Fatalf("kabar lunas: %+v, %v", u, err)
	}
	if got := f.stock(t, f.keripik); got != 9 {
		t.Errorf("stok setelah lunas = %d; want 9", got)
	}
	if u, err := f.svc.ApplyGatewayResult(ctx, ref, "paid", 10000, nil); u != nil || err != nil {
		t.Errorf("kabar ulang: %+v, %v", u, err)
	}
	if got := f.stock(t, f.keripik); got != 9 {
		t.Errorf("stok setelah kabar ulang = %d; want 9", got)
	}
}

func TestSwitchFromQRIS(t *testing.T) {
	f := setup(t)
	ctx := context.Background()

	id, err := f.svc.Checkout(ctx, f.userID, CheckoutInput{
		ClientRef: "7b1f5f0e-0000-4000-8000-000000000007",
		Lines:     []CartLine{{ProductID: f.keripik, Qty: 1}},
		Payments:  []PaymentInput{{Method: "qris", Amount: 15000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	charges, _ := f.svc.UnchargedQRIS(ctx, id)
	ref := charges[0].ProviderRef

	if _, err := f.svc.AddPayments(ctx, id, []PaymentInput{{Method: "cash", Amount: 14000}}); !errors.Is(err, ErrPaymentMismatch) {
		t.Errorf("sisa tidak pas: err = %v", err)
	}
	dropped, err := f.svc.AddPayments(ctx, id, []PaymentInput{{Method: "cash", Amount: 15000}})
	if err != nil || len(dropped) != 1 || dropped[0] != ref {
		t.Fatalf("AddPayments = %v, %v", dropped, err)
	}
	sale, _ := f.svc.Get(ctx, id)
	if sale.Status != "paid" || len(sale.Payments) != 2 || sale.Payments[0].Status != "failed" {
		t.Errorf("sale = %+v", sale)
	}
	// Pembeli tetap membayar QR lama: tidak boleh melunasi atau mengurangi stok lagi.
	if u, err := f.svc.ApplyGatewayResult(ctx, ref, "paid", 15000, nil); u != nil || err != nil {
		t.Errorf("QR lama dibayar setelah diganti: %+v, %v", u, err)
	}
	if got := f.stock(t, f.keripik); got != 9 {
		t.Errorf("stok = %d; want 9", got)
	}
	if _, err := f.svc.AddPayments(ctx, id, []PaymentInput{{Method: "cash", Amount: 1}}); !errors.Is(err, ErrNotPending) {
		t.Errorf("menambah bayar ke transaksi lunas: err = %v", err)
	}
}

func TestDailyRecap(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	tendered := int64(20000)
	ref := "APPR1"
	sell := func(n int, productID int64, payments ...PaymentInput) int64 {
		id, err := f.svc.Checkout(ctx, f.userID, CheckoutInput{
			ClientRef: "7b1f5f0e-0000-4000-8000-00000000010" + string(rune('0'+n)),
			Lines:     []CartLine{{ProductID: productID, Qty: 1}},
			Payments:  payments,
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	// Tunai 15.000; split debit 10.000 + tunai 5.000; satu penjualan tunai 4.000 yang dibatalkan;
	// satu QRIS yang belum dibayar.
	sell(1, f.keripik, PaymentInput{Method: "cash", Amount: 15000, Tendered: &tendered})
	sell(2, f.keripik, PaymentInput{Method: "debit", Amount: 10000, Reference: &ref}, PaymentInput{Method: "cash", Amount: 5000})
	voided := sell(3, f.air, PaymentInput{Method: "cash", Amount: 4000})
	if _, err := f.svc.Void(ctx, voided, f.userID); err != nil {
		t.Fatal(err)
	}
	sell(4, f.air, PaymentInput{Method: "qris", Amount: 4000})

	today := time.Now().Format(time.DateOnly)
	r, err := f.svc.DailyRecap(ctx, today)
	if err != nil {
		t.Fatal(err)
	}
	if r.Transactions != 2 || r.Voided != 1 || r.Cash != 20000 || r.Debit != 10000 || r.QRIS != 0 || r.Total != 30000 {
		t.Errorf("rekap = %+v", r)
	}
	yesterday := time.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	if r, _ := f.svc.DailyRecap(ctx, yesterday); r.Transactions != 0 || r.Total != 0 {
		t.Errorf("rekap kemarin = %+v", r)
	}
}
