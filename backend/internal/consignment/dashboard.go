package consignment

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// Stok aktif pada atau di bawah angka ini masuk daftar "menipis".
const lowStockAt = 3

type DayTotal struct {
	Day   time.Time `json:"day"`
	Gross int64     `json:"gross"`
}

type ProductCount struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type Dashboard struct {
	NoPrice             int            `json:"no_price"`
	InactiveWithStock   int            `json:"inactive_with_stock"`
	OutOfStock          int            `json:"out_of_stock"`
	NoBarcode           int            `json:"no_barcode"`
	PendingTransactions int            `json:"pending_transactions"`
	OutstandingPayout   int64          `json:"outstanding_payout"`
	UnpaidSettlements   int64          `json:"unpaid_settlements"`
	DraftOpnames        int            `json:"draft_opnames"`
	Week                []DayTotal     `json:"week"`
	LowStock            []ProductCount `json:"low_stock"`
	TopProducts         []ProductCount `json:"top_products"`
}

// Dashboard mengumpulkan hal-hal yang perlu dilihat pemilik toko setiap hari dalam satu panggilan.
func (s *Service) Dashboard(ctx context.Context) (*Dashboard, error) {
	var d Dashboard
	err := s.db.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM products WHERE price = 0),
		       (SELECT count(*) FROM products WHERE NOT is_active AND stock > 0),
		       (SELECT count(*) FROM products WHERE is_active AND stock <= 0),
		       (SELECT count(*) FROM products WHERE barcode IS NULL),
		       (SELECT count(*) FROM transactions WHERE status = 'pending'),
		       (SELECT COALESCE(sum(ti.vendor_amount), 0)
		        FROM transaction_items ti JOIN transactions t ON t.id = ti.transaction_id
		        WHERE t.status = 'paid' AND ti.member_id IS NOT NULL AND ti.settlement_id IS NULL),
		       (SELECT COALESCE(sum(payout_amount), 0) FROM vendor_settlements WHERE status = 'draft'),
		       (SELECT count(*) FROM stock_opnames WHERE status = 'draft')`).
		Scan(&d.NoPrice, &d.InactiveWithStock, &d.OutOfStock, &d.NoBarcode, &d.PendingTransactions,
			&d.OutstandingPayout, &d.UnpaidSettlements, &d.DraftOpnames)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT day::date, COALESCE(sum(t.total), 0)
		FROM generate_series(current_date - 6, current_date, interval '1 day') AS day
		LEFT JOIN transactions t ON t.status = 'paid' AND t.paid_at >= day AND t.paid_at < day + interval '1 day'
		GROUP BY day ORDER BY day`)
	if err != nil {
		return nil, err
	}
	if d.Week, err = pgx.CollectRows(rows, pgx.RowToStructByPos[DayTotal]); err != nil {
		return nil, err
	}

	rows, err = s.db.Query(ctx, `
		SELECT name, stock FROM products
		WHERE is_active AND stock <= $1
		ORDER BY stock, name LIMIT 10`, lowStockAt)
	if err != nil {
		return nil, err
	}
	if d.LowStock, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ProductCount]); err != nil {
		return nil, err
	}

	rows, err = s.db.Query(ctx, `
		SELECT ti.product_name, sum(ti.qty)::int
		FROM transaction_items ti
		JOIN transactions t ON t.id = ti.transaction_id
		WHERE t.status = 'paid' AND t.paid_at >= current_date - 6
		GROUP BY ti.product_name
		ORDER BY sum(ti.qty) DESC, ti.product_name LIMIT 5`)
	if err != nil {
		return nil, err
	}
	d.TopProducts, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ProductCount])
	return &d, err
}
