package sales

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidCart     = errors.New("keranjang tidak valid")
	ErrProductNotFound = errors.New("ada produk yang tidak ditemukan atau sudah nonaktif; muat ulang katalog")
	ErrPaymentMismatch = errors.New("jumlah pembayaran tidak sama dengan total")
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

type CartLine struct {
	ProductID int64 `json:"product_id"`
	Qty       int   `json:"qty"`
}

type PaymentInput struct {
	Method    string  `json:"method"`
	Amount    int64   `json:"amount"`
	Tendered  *int64  `json:"tendered"`
	Reference *string `json:"reference"`
}

type CheckoutInput struct {
	ClientRef string         `json:"client_ref"`
	Lines     []CartLine     `json:"lines"`
	Payments  []PaymentInput `json:"payments"`
}

type product struct {
	ID            int64
	Name          string
	Price         int64
	MemberID      *int64
	CommissionBps int
}

// Checkout mengembalikan id transaksi. client_ref yang sama selalu menghasilkan transaksi yang sama,
// jadi tombol bayar yang tertekan dua kali atau request yang diulang tidak membuat penjualan ganda.
func (s *Service) Checkout(ctx context.Context, cashierID int64, in CheckoutInput) (int64, error) {
	if len(in.Lines) == 0 || len(in.Payments) == 0 {
		return 0, ErrInvalidCart
	}
	qty := make(map[int64]int, len(in.Lines))
	ids := make([]int64, 0, len(in.Lines))
	for _, l := range in.Lines {
		if l.Qty <= 0 {
			return 0, ErrInvalidCart
		}
		if _, seen := qty[l.ProductID]; !seen {
			ids = append(ids, l.ProductID)
		}
		qty[l.ProductID] += l.Qty
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT p.id, p.name, p.price, p.member_id,
		       COALESCE(p.commission_bps, m.commission_bps, 10000)
		FROM products p
		LEFT JOIN members m ON m.id = p.member_id
		WHERE p.id = ANY($1) AND p.is_active`, ids)
	if err != nil {
		return 0, err
	}
	products, err := pgx.CollectRows(rows, pgx.RowToStructByPos[product])
	if err != nil {
		return 0, err
	}
	if len(products) != len(ids) {
		return 0, ErrProductNotFound
	}

	var total int64
	items := make([][]any, 0, len(products))
	for _, p := range products {
		q := qty[p.ID]
		lineTotal := p.Price * int64(q)
		store, vendor := splitCommission(lineTotal, p.CommissionBps)
		total += lineTotal
		items = append(items, []any{nil, p.ID, p.MemberID, p.Name, p.Price, q, lineTotal, p.CommissionBps, store, vendor})
	}

	var paying int64
	for _, p := range in.Payments {
		paying += p.Amount
	}
	if paying != total {
		return 0, ErrPaymentMismatch
	}

	var id int64
	var invoiceNo string
	err = tx.QueryRow(ctx, `
		INSERT INTO transactions (client_ref, cashier_id, total)
		VALUES ($1, $2, $3)
		ON CONFLICT (client_ref) DO NOTHING
		RETURNING id, invoice_no`, in.ClientRef, cashierID, total).Scan(&id, &invoiceNo)
	if errors.Is(err, pgx.ErrNoRows) {
		err = s.db.QueryRow(ctx, `SELECT id FROM transactions WHERE client_ref = $1`, in.ClientRef).Scan(&id)
		return id, err
	}
	if err != nil {
		return 0, err
	}

	for i := range items {
		items[i][0] = id
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"transaction_items"},
		[]string{"transaction_id", "product_id", "member_id", "product_name", "unit_price", "qty",
			"line_total", "commission_bps", "commission_amount", "vendor_amount"},
		pgx.CopyFromRows(items))
	if err != nil {
		return 0, err
	}

	if err := insertPayments(ctx, tx, id, invoiceNo, 0, in.Payments); err != nil {
		return 0, err
	}
	if _, err := settleIfCovered(ctx, tx, id); err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

// insertPayments mencatat tunai dan debit sebagai lunas; QRIS menunggu kabar dari gateway.
// existing = jumlah pembayaran yang sudah ada, supaya provider_ref (order_id di gateway) tetap unik.
func insertPayments(ctx context.Context, tx pgx.Tx, transactionID int64, invoiceNo string, existing int, payments []PaymentInput) error {
	for i, p := range payments {
		status := "paid"
		var providerRef *string
		if p.Method == "qris" {
			ref := fmt.Sprintf("%s-%d", invoiceNo, existing+i+1)
			status, providerRef = "pending", &ref
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO payments (transaction_id, method, amount, tendered, reference, status, provider_ref, paid_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, CASE WHEN $6 = 'paid' THEN now() END)`,
			transactionID, p.Method, p.Amount, p.Tendered, p.Reference, status, providerRef)
		if err != nil {
			return err
		}
	}
	return nil
}

// settleIfCovered melunasi transaksi bila total payments berstatus paid sudah menutup total.
// Baris transactions dikunci dulu supaya dua pembayaran yang selesai bersamaan tidak saling melewatkan.
func settleIfCovered(ctx context.Context, tx pgx.Tx, transactionID int64) (bool, error) {
	var total int64
	var status string
	err := tx.QueryRow(ctx,
		`SELECT total, status FROM transactions WHERE id = $1 FOR UPDATE`, transactionID).Scan(&total, &status)
	if err != nil {
		return false, err
	}
	if status != "pending" {
		return false, nil
	}

	var paid int64
	err = tx.QueryRow(ctx,
		`SELECT COALESCE(sum(amount), 0) FROM payments WHERE transaction_id = $1 AND status = 'paid'`,
		transactionID).Scan(&paid)
	if err != nil || paid < total {
		return false, err
	}

	_, err = tx.Exec(ctx, `UPDATE transactions SET status = 'paid', paid_at = now() WHERE id = $1`, transactionID)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `
		WITH sold AS (
			SELECT product_id, sum(qty)::int AS qty
			FROM transaction_items WHERE transaction_id = $1
			GROUP BY product_id
		), deducted AS (
			UPDATE products p SET stock = p.stock - sold.qty
			FROM sold WHERE p.id = sold.product_id
		)
		INSERT INTO stock_movements (product_id, qty_change, reason, ref_id)
		SELECT product_id, -qty, 'sale', $1 FROM sold`, transactionID)
	return err == nil, err
}
