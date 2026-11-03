package sales

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotFound    = errors.New("transaksi tidak ditemukan")
	ErrAlreadyVoid = errors.New("transaksi sudah dibatalkan")
	ErrSettled     = errors.New("transaksi sudah masuk pembagian hasil dan tidak bisa dibatalkan")
)

type Item struct {
	Name      string `json:"name"`
	Qty       int    `json:"qty"`
	UnitPrice int64  `json:"unit_price"`
	LineTotal int64  `json:"line_total"`
}

type Payment struct {
	ID        int64  `json:"id"`
	Method    string `json:"method"`
	Amount    int64  `json:"amount"`
	Tendered  *int64 `json:"tendered"`
	Reference string `json:"reference"`
	Status    string `json:"status"`
	QRString  string `json:"qr_string"`
}

type Transaction struct {
	ID        int64      `json:"id"`
	InvoiceNo string     `json:"invoice_no"`
	Status    string     `json:"status"`
	Total     int64      `json:"total"`
	CreatedAt time.Time  `json:"created_at"`
	PaidAt    *time.Time `json:"paid_at"`
	Cashier   string     `json:"cashier"`
	Methods   string     `json:"methods"`
	Items     []Item     `json:"items,omitempty" db:"-"`
	Payments  []Payment  `json:"payments,omitempty" db:"-"`
}

const transactionSelect = `
	SELECT t.id, t.invoice_no, t.status, t.total, t.created_at, t.paid_at, u.full_name,
	       COALESCE((SELECT string_agg(DISTINCT method, ',') FROM payments
	                 WHERE transaction_id = t.id AND status IN ('paid', 'pending')), '')
	FROM transactions t
	JOIN users u ON u.id = t.cashier_id`

// ListByDate menerima tanggal kalender toko ("2026-10-12"); batas harinya mengikuti zona waktu koneksi.
func (s *Service) ListByDate(ctx context.Context, date string) ([]Transaction, error) {
	rows, err := s.db.Query(ctx, transactionSelect+`
		WHERE t.created_at >= $1::date AND t.created_at < $1::date + 1
		ORDER BY t.id DESC`, date)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Transaction])
}

type Recap struct {
	Date         string `json:"date"`
	Transactions int    `json:"transactions"`
	Voided       int    `json:"voided"`
	Cash         int64  `json:"cash"`
	Debit        int64  `json:"debit"`
	QRIS         int64  `json:"qris"`
	Total        int64  `json:"total"`
}

// DailyRecap merangkum transaksi yang lunas pada satu tanggal kalender toko, per cara bayar.
// Transaksi yang dibatalkan tidak dihitung; uangnya dianggap sudah dikembalikan.
func (s *Service) DailyRecap(ctx context.Context, date string) (*Recap, error) {
	r := Recap{Date: date}
	err := s.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM transactions
		        WHERE status = 'paid' AND paid_at >= $1::date AND paid_at < $1::date + 1),
		       (SELECT count(*) FROM transactions
		        WHERE status = 'void' AND voided_at >= $1::date AND voided_at < $1::date + 1),
		       COALESCE(sum(p.amount) FILTER (WHERE p.method = 'cash'), 0),
		       COALESCE(sum(p.amount) FILTER (WHERE p.method = 'debit'), 0),
		       COALESCE(sum(p.amount) FILTER (WHERE p.method = 'qris'), 0),
		       COALESCE(sum(p.amount), 0)
		FROM payments p
		JOIN transactions t ON t.id = p.transaction_id
		WHERE t.status = 'paid' AND p.status = 'paid'
		  AND t.paid_at >= $1::date AND t.paid_at < $1::date + 1`, date).
		Scan(&r.Transactions, &r.Voided, &r.Cash, &r.Debit, &r.QRIS, &r.Total)
	return &r, err
}

func (s *Service) Get(ctx context.Context, id int64) (*Transaction, error) {
	rows, err := s.db.Query(ctx, transactionSelect+` WHERE t.id = $1`, id)
	if err != nil {
		return nil, err
	}
	t, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByPos[Transaction])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err = s.db.Query(ctx, `
		SELECT product_name, qty, unit_price, line_total
		FROM transaction_items WHERE transaction_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	if t.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Item]); err != nil {
		return nil, err
	}

	rows, err = s.db.Query(ctx, `
		SELECT id, method, amount, tendered, COALESCE(reference, ''), status, COALESCE(qr_string, '')
		FROM payments WHERE transaction_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	t.Payments, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Payment])
	return t, err
}

// Void membatalkan transaksi. Stok hanya dikembalikan bila transaksi sudah lunas,
// karena transaksi pending belum pernah mengurangi stok. Yang dikembalikan adalah
// provider_ref tagihan QRIS yang masih menunggu, untuk dibatalkan di gateway.
func (s *Service) Void(ctx context.Context, id, userID int64) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM transactions WHERE id = $1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status == "void" {
		return nil, ErrAlreadyVoid
	}

	var settled bool
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(bool_or(settlement_id IS NOT NULL), FALSE)
		FROM (SELECT settlement_id FROM transaction_items WHERE transaction_id = $1 FOR UPDATE) locked`, id).Scan(&settled)
	if err != nil {
		return nil, err
	}
	if settled {
		return nil, ErrSettled
	}

	if _, err = tx.Exec(ctx, `UPDATE transactions SET status = 'void', voided_at = now() WHERE id = $1`, id); err != nil {
		return nil, err
	}
	open, err := failPendingPayments(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if status == "paid" {
		_, err = tx.Exec(ctx, `
			WITH sold AS (
				SELECT product_id, sum(qty)::int AS qty
				FROM transaction_items WHERE transaction_id = $1
				GROUP BY product_id
			), restored AS (
				UPDATE products p SET stock = p.stock + sold.qty
				FROM sold WHERE p.id = sold.product_id
			)
			INSERT INTO stock_movements (product_id, qty_change, reason, ref_id, user_id)
			SELECT product_id, qty, 'void', $1, $2 FROM sold`, id, userID)
		if err != nil {
			return nil, err
		}
	}
	return open, tx.Commit(ctx)
}
