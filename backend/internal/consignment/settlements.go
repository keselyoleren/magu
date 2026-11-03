package consignment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrSettlementNotFound = errors.New("pembagian hasil tidak ditemukan")
	ErrSettlementPaid     = errors.New("pembagian hasil sudah dibayar")
	ErrNothingToSettle    = errors.New("tidak ada penjualan yang belum dibagi sampai tanggal itu")
)

type Outstanding struct {
	MemberID        int64     `json:"member_id"`
	MemberCode      string    `json:"member_code"`
	MemberName      string    `json:"member_name"`
	GrossSales      int64     `json:"gross_sales"`
	CommissionTotal int64     `json:"commission_total"`
	PayoutAmount    int64     `json:"payout_amount"`
	Since           time.Time `json:"since"`
}

// Outstanding merangkum penjualan lunas yang belum masuk pembagian hasil, per member.
func (s *Service) Outstanding(ctx context.Context) ([]Outstanding, error) {
	rows, err := s.db.Query(ctx, `
		SELECT m.id, m.code, m.name, sum(ti.line_total), sum(ti.commission_amount), sum(ti.vendor_amount), min(t.paid_at)
		FROM transaction_items ti
		JOIN transactions t ON t.id = ti.transaction_id AND t.status = 'paid'
		JOIN members m ON m.id = ti.member_id
		WHERE ti.settlement_id IS NULL
		GROUP BY m.id
		ORDER BY sum(ti.vendor_amount) DESC`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Outstanding])
}

type SettlementLine struct {
	PaidAt           time.Time `json:"paid_at"`
	InvoiceNo        string    `json:"invoice_no"`
	ProductName      string    `json:"product_name"`
	Qty              int       `json:"qty"`
	LineTotal        int64     `json:"line_total"`
	CommissionBps    int       `json:"commission_bps"`
	CommissionAmount int64     `json:"commission_amount"`
	VendorAmount     int64     `json:"vendor_amount"`
}

type Settlement struct {
	ID              int64            `json:"id"`
	MemberID        int64            `json:"member_id"`
	MemberCode      string           `json:"member_code"`
	MemberName      string           `json:"member_name"`
	Bank            string           `json:"bank"`
	PeriodStart     time.Time        `json:"period_start"`
	PeriodEnd       time.Time        `json:"period_end"`
	GrossSales      int64            `json:"gross_sales"`
	CommissionTotal int64            `json:"commission_total"`
	PayoutAmount    int64            `json:"payout_amount"`
	Status          string           `json:"status"`
	CreatedAt       time.Time        `json:"created_at"`
	PaidAt          *time.Time       `json:"paid_at"`
	DriveSentAt     *time.Time       `json:"drive_sent_at"`
	Lines           []SettlementLine `json:"lines,omitempty" db:"-"`
}

const settlementSelect = `
	SELECT vs.id, m.id, m.code, m.name,
	       concat_ws(' · ', m.bank_name, m.bank_account_number, m.bank_account_name),
	       vs.period_start, vs.period_end, vs.gross_sales, vs.commission_total, vs.payout_amount,
	       vs.status, vs.created_at, vs.paid_at, vs.drive_sent_at
	FROM vendor_settlements vs
	JOIN members m ON m.id = vs.member_id`

func (s *Service) ListSettlements(ctx context.Context) ([]Settlement, error) {
	rows, err := s.db.Query(ctx, settlementSelect+` ORDER BY vs.id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Settlement])
}

func (s *Service) GetSettlement(ctx context.Context, id int64) (*Settlement, error) {
	rows, err := s.db.Query(ctx, settlementSelect+` WHERE vs.id = $1`, id)
	if err != nil {
		return nil, err
	}
	st, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByPos[Settlement])
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSettlementNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err = s.db.Query(ctx, `
		SELECT t.paid_at, t.invoice_no, ti.product_name, ti.qty, ti.line_total,
		       ti.commission_bps, ti.commission_amount, ti.vendor_amount
		FROM transaction_items ti
		JOIN transactions t ON t.id = ti.transaction_id
		WHERE ti.settlement_id = $1
		ORDER BY t.paid_at, ti.id`, id)
	if err != nil {
		return nil, err
	}
	st.Lines, err = pgx.CollectRows(rows, pgx.RowToStructByPos[SettlementLine])
	return st, err
}

// CreateSettlement mengklaim semua penjualan lunas milik member yang belum dibagi sampai akhir hari `until`.
func (s *Service) CreateSettlement(ctx context.Context, userID, memberID int64, until time.Time) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	// Kunci baris transaksinya dulu supaya void yang berjalan bersamaan tidak lolos:
	// void juga mengunci baris yang sama, jadi salah satu pasti melihat hasil yang lain.
	_, err = tx.Exec(ctx, `
		SELECT 1 FROM transactions t
		WHERE t.status = 'paid' AND t.paid_at < $2::date + 1
		  AND EXISTS (SELECT 1 FROM transaction_items ti
		              WHERE ti.transaction_id = t.id AND ti.member_id = $1 AND ti.settlement_id IS NULL)
		FOR UPDATE OF t`, memberID, until)
	if err != nil {
		return 0, err
	}

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO vendor_settlements (member_id, period_start, period_end, created_by)
		VALUES ($1, $2, $2, $3) RETURNING id`, memberID, until, userID).Scan(&id)
	if err != nil {
		return 0, memberError(err)
	}

	var gross, commission, payout int64
	var first *time.Time
	err = tx.QueryRow(ctx, `
		WITH claimed AS (
			UPDATE transaction_items ti SET settlement_id = $1
			FROM transactions t
			WHERE t.id = ti.transaction_id
			  AND t.status = 'paid'
			  AND t.paid_at < $3::date + 1
			  AND ti.member_id = $2
			  AND ti.settlement_id IS NULL
			RETURNING ti.line_total, ti.commission_amount, ti.vendor_amount, t.paid_at
		)
		SELECT COALESCE(sum(line_total), 0), COALESCE(sum(commission_amount), 0),
		       COALESCE(sum(vendor_amount), 0), min(paid_at)
		FROM claimed`, id, memberID, until).Scan(&gross, &commission, &payout, &first)
	if err != nil {
		return 0, err
	}
	if first == nil {
		return 0, ErrNothingToSettle
	}

	_, err = tx.Exec(ctx, `
		UPDATE vendor_settlements
		SET gross_sales = $2, commission_total = $3, payout_amount = $4, period_start = $5::date
		WHERE id = $1`, id, gross, commission, payout, *first)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (s *Service) PaySettlement(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE vendor_settlements SET status = 'paid', paid_at = now()
		WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.settlementDraftError(ctx, id)
	}
	return nil
}

// DeleteSettlement menghapus draft dan melepas kembali baris penjualannya supaya bisa dibagi ulang.
func (s *Service) DeleteSettlement(ctx context.Context, id int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM vendor_settlements WHERE id = $1 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSettlementNotFound
	}
	if err != nil {
		return err
	}
	if status != "draft" {
		return ErrSettlementPaid
	}
	if _, err = tx.Exec(ctx, `UPDATE transaction_items SET settlement_id = NULL WHERE settlement_id = $1`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM vendor_settlements WHERE id = $1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) settlementDraftError(ctx context.Context, id int64) error {
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vendor_settlements WHERE id = $1)`, id).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrSettlementPaid
	}
	return ErrSettlementNotFound
}

type DailySales struct {
	Day          time.Time `json:"day"`
	Transactions int       `json:"transactions"`
	Gross        int64     `json:"gross"`
	StoreShare   int64     `json:"store_share"`
	VendorShare  int64     `json:"vendor_share"`
	Cash         int64     `json:"cash"`
	Debit        int64     `json:"debit"`
	QRIS         int64     `json:"qris"`
}

type OwnerSales struct {
	MemberID    *int64 `json:"member_id"`
	Name        string `json:"name"`
	Qty         int    `json:"qty"`
	Gross       int64  `json:"gross"`
	StoreShare  int64  `json:"store_share"`
	VendorShare int64  `json:"vendor_share"`
}

// SalesReport menjumlahkan angka yang tersimpan di transaction_items; komisi tidak pernah dihitung ulang.
func (s *Service) SalesReport(ctx context.Context, from, to time.Time) ([]DailySales, []OwnerSales, error) {
	rows, err := s.db.Query(ctx, `
		WITH sold AS (
			SELECT t.paid_at::date AS day, count(DISTINCT t.id) AS transactions, sum(ti.line_total) AS gross,
			       sum(ti.commission_amount) AS store_share, sum(ti.vendor_amount) AS vendor_share
			FROM transactions t
			JOIN transaction_items ti ON ti.transaction_id = t.id
			WHERE t.status = 'paid' AND t.paid_at >= $1::date AND t.paid_at < $2::date + 1
			GROUP BY 1
		), paid AS (
			SELECT t.paid_at::date AS day,
			       COALESCE(sum(p.amount) FILTER (WHERE p.method = 'cash'), 0) AS cash,
			       COALESCE(sum(p.amount) FILTER (WHERE p.method = 'debit'), 0) AS debit,
			       COALESCE(sum(p.amount) FILTER (WHERE p.method = 'qris'), 0) AS qris
			FROM transactions t
			JOIN payments p ON p.transaction_id = t.id AND p.status = 'paid'
			WHERE t.status = 'paid' AND t.paid_at >= $1::date AND t.paid_at < $2::date + 1
			GROUP BY 1
		)
		SELECT sold.day, sold.transactions, sold.gross, sold.store_share, sold.vendor_share,
		       paid.cash, paid.debit, paid.qris
		FROM sold JOIN paid USING (day)
		ORDER BY sold.day`, from, to)
	if err != nil {
		return nil, nil, err
	}
	daily, err := pgx.CollectRows(rows, pgx.RowToStructByPos[DailySales])
	if err != nil {
		return nil, nil, err
	}

	rows, err = s.db.Query(ctx, `
		SELECT ti.member_id, COALESCE(m.name, ''), sum(ti.qty), sum(ti.line_total),
		       sum(ti.commission_amount), sum(ti.vendor_amount)
		FROM transactions t
		JOIN transaction_items ti ON ti.transaction_id = t.id
		LEFT JOIN members m ON m.id = ti.member_id
		WHERE t.status = 'paid' AND t.paid_at >= $1::date AND t.paid_at < $2::date + 1
		GROUP BY ti.member_id, m.name
		ORDER BY sum(ti.line_total) DESC`, from, to)
	if err != nil {
		return nil, nil, err
	}
	owners, err := pgx.CollectRows(rows, pgx.RowToStructByPos[OwnerSales])
	return daily, owners, err
}
