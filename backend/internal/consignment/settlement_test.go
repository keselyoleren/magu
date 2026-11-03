package consignment

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"magu/internal/sales"
)

func (f *fixture) sell(t *testing.T, n int, lines ...sales.CartLine) int64 {
	var total int64
	for _, l := range lines {
		var price int64
		if err := f.db.QueryRow(context.Background(), `SELECT price FROM products WHERE id = $1`, l.ProductID).Scan(&price); err != nil {
			t.Fatal(err)
		}
		total += price * int64(l.Qty)
	}
	id, err := sales.NewService(f.db).Checkout(context.Background(), f.userID, sales.CheckoutInput{
		ClientRef: fmt.Sprintf("7b1f5f0e-0000-4000-8000-%012d", n),
		Lines:     lines,
		Payments:  []sales.PaymentInput{{Method: "cash", Amount: total}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSettlement(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	today := time.Now()

	// Keripik 15.000 dan Sambal 9.000 milik member (komisi 20%); Air 4.000 milik toko.
	first := f.sell(t, 1, sales.CartLine{ProductID: f.keripik, Qty: 2}, sales.CartLine{ProductID: f.air, Qty: 1})
	f.sell(t, 2, sales.CartLine{ProductID: f.sambal, Qty: 1})

	out, err := f.svc.Outstanding(ctx)
	if err != nil || len(out) != 1 || out[0].GrossSales != 39000 || out[0].CommissionTotal != 7800 || out[0].PayoutAmount != 31200 {
		t.Fatalf("Outstanding = %+v, %v", out, err)
	}

	if _, err := f.svc.CreateSettlement(ctx, f.userID, f.memberID, today.AddDate(0, 0, -1)); !errors.Is(err, ErrNothingToSettle) {
		t.Errorf("batas kemarin: err = %v", err)
	}
	id, err := f.svc.CreateSettlement(ctx, f.userID, f.memberID, today)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.svc.GetSettlement(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if st.GrossSales != 39000 || st.CommissionTotal != 7800 || st.PayoutAmount != 31200 || len(st.Lines) != 2 || st.Status != "draft" {
		t.Errorf("settlement = %+v", st)
	}
	if _, err := f.svc.CreateSettlement(ctx, f.userID, f.memberID, today); !errors.Is(err, ErrNothingToSettle) {
		t.Errorf("pembagian kedua: err = %v", err)
	}
	if out, _ := f.svc.Outstanding(ctx); len(out) != 0 {
		t.Errorf("masih ada yang belum dibagi: %+v", out)
	}
	if _, err := sales.NewService(f.db).Void(ctx, first, f.userID); !errors.Is(err, sales.ErrSettled) {
		t.Errorf("void transaksi yang sudah dibagi: err = %v", err)
	}

	if err := f.svc.DeleteSettlement(ctx, id); err != nil {
		t.Fatal(err)
	}
	if out, _ := f.svc.Outstanding(ctx); len(out) != 1 || out[0].PayoutAmount != 31200 {
		t.Errorf("setelah draft dihapus: %+v", out)
	}

	id, err = f.svc.CreateSettlement(ctx, f.userID, f.memberID, today)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.svc.PaySettlement(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.PaySettlement(ctx, id); !errors.Is(err, ErrSettlementPaid) {
		t.Errorf("bayar dua kali: err = %v", err)
	}
	if err := f.svc.DeleteSettlement(ctx, id); !errors.Is(err, ErrSettlementPaid) {
		t.Errorf("hapus yang sudah dibayar: err = %v", err)
	}

	d, err := f.svc.Dashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Tiga produk uji berharga, aktif; Sambal dan Air belum pernah diterima sehingga stoknya minus setelah terjual.
	if d.NoPrice != 0 || d.OutstandingPayout != 0 || d.UnpaidSettlements != 0 || len(d.Week) != 7 ||
		d.Week[6].Gross != 43000 || len(d.TopProducts) != 3 || d.TopProducts[0].Name != "Keripik" || d.TopProducts[0].Count != 2 ||
		d.OutOfStock != 3 || len(d.LowStock) != 3 {
		t.Errorf("dashboard = %+v", d)
	}

	daily, owners, err := f.svc.SalesReport(ctx, today, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 1 || daily[0].Transactions != 2 || daily[0].Gross != 43000 || daily[0].StoreShare != 11800 || daily[0].VendorShare != 31200 ||
		daily[0].Cash != 43000 || daily[0].Debit != 0 {
		t.Errorf("daily = %+v", daily)
	}
	if len(owners) != 2 || owners[0].Name != "Bu Sari" || owners[0].Gross != 39000 || owners[1].MemberID != nil || owners[1].StoreShare != 4000 {
		t.Errorf("owners = %+v", owners)
	}
}
