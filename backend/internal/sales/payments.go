package sales

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

var (
	ErrNotPending      = errors.New("transaksi tidak sedang menunggu pembayaran")
	ErrPaymentNotFound = errors.New("pembayaran tidak ditemukan")
)

type PaymentUpdate struct {
	PaymentID       int64  `json:"payment_id"`
	TransactionID   int64  `json:"transaction_id"`
	PaymentStatus   string `json:"payment_status"`
	TransactionPaid bool   `json:"transaction_paid"`
}

// ApplyGatewayResult dipakai webhook dan tombol "Cek status". Mengembalikan nil bila tidak ada yang berubah:
// provider_ref tidak dikenal, nominal tidak cocok, atau kabar yang sama datang lagi.
func (s *Service) ApplyGatewayResult(ctx context.Context, providerRef, status string, amount int64, payload []byte) (*PaymentUpdate, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	u := &PaymentUpdate{PaymentStatus: status}
	err = tx.QueryRow(ctx, `
		UPDATE payments
		SET status = $2, paid_at = CASE WHEN $2 = 'paid' THEN now() END, gateway_payload = $4
		WHERE provider_ref = $1 AND status = 'pending' AND amount = $3
		RETURNING id, transaction_id`, providerRef, status, amount, payload).Scan(&u.PaymentID, &u.TransactionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if status == "paid" {
		if u.TransactionPaid, err = settleIfCovered(ctx, tx, u.TransactionID); err != nil {
			return nil, err
		}
	}
	return u, tx.Commit(ctx)
}

type Charge struct {
	PaymentID   int64
	ProviderRef string
	Amount      int64
}

// UnchargedQRIS mengembalikan pembayaran QRIS yang tagihannya belum dibuat di gateway.
func (s *Service) UnchargedQRIS(ctx context.Context, transactionID int64) ([]Charge, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, provider_ref, amount FROM payments
		WHERE transaction_id = $1 AND method = 'qris' AND status = 'pending' AND qr_string IS NULL`, transactionID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Charge])
}

func (s *Service) SetQRString(ctx context.Context, paymentID int64, qr string) error {
	_, err := s.db.Exec(ctx, `UPDATE payments SET qr_string = $2 WHERE id = $1`, paymentID, qr)
	return err
}

func (s *Service) FailPayment(ctx context.Context, paymentID int64) error {
	_, err := s.db.Exec(ctx, `UPDATE payments SET status = 'failed' WHERE id = $1 AND status = 'pending'`, paymentID)
	return err
}

func (s *Service) PendingCharge(ctx context.Context, paymentID int64) (*Charge, error) {
	c := Charge{PaymentID: paymentID}
	err := s.db.QueryRow(ctx, `
		SELECT provider_ref, amount FROM payments
		WHERE id = $1 AND method = 'qris' AND status = 'pending'`, paymentID).Scan(&c.ProviderRef, &c.Amount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPaymentNotFound
	}
	return &c, err
}

// AddPayments melunasi sisa transaksi pending dengan cara bayar lain, mis. setelah QRIS kedaluwarsa.
// Tagihan QRIS yang masih menunggu digugurkan; provider_ref-nya dikembalikan untuk dibatalkan di gateway.
func (s *Service) AddPayments(ctx context.Context, transactionID int64, payments []PaymentInput) ([]string, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var total int64
	var status, invoiceNo string
	err = tx.QueryRow(ctx, `
		SELECT total, status, invoice_no FROM transactions WHERE id = $1 FOR UPDATE`, transactionID).
		Scan(&total, &status, &invoiceNo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != "pending" {
		return nil, ErrNotPending
	}

	dropped, err := failPendingPayments(ctx, tx, transactionID)
	if err != nil {
		return nil, err
	}

	var paid int64
	var existing int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(sum(amount) FILTER (WHERE status = 'paid'), 0), count(*)
		FROM payments WHERE transaction_id = $1`, transactionID).Scan(&paid, &existing)
	if err != nil {
		return nil, err
	}
	var paying int64
	for _, p := range payments {
		paying += p.Amount
	}
	if paying != total-paid {
		return nil, ErrPaymentMismatch
	}

	if err := insertPayments(ctx, tx, transactionID, invoiceNo, existing, payments); err != nil {
		return nil, err
	}
	if _, err := settleIfCovered(ctx, tx, transactionID); err != nil {
		return nil, err
	}
	return dropped, tx.Commit(ctx)
}

func failPendingPayments(ctx context.Context, tx pgx.Tx, transactionID int64) ([]string, error) {
	rows, err := tx.Query(ctx, `
		UPDATE payments SET status = 'failed'
		WHERE transaction_id = $1 AND status = 'pending'
		RETURNING provider_ref`, transactionID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
