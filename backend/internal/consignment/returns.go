package consignment

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
)

var ErrReturnNotFound = errors.New("retur tidak ditemukan")

// Retur ke penitip memakai bentuk data yang sama dengan penerimaan (Receipt, ReceiptLine):
// received_at dan received_by berisi waktu dan petugas retur.
const returnSelect = `
	SELECT r.id, r.member_id, m.name, COALESCE(r.note, ''), r.returned_at, u.full_name,
	       count(i.id), COALESCE(sum(i.qty), 0), NULL::timestamptz, ''::text
	FROM vendor_returns r
	JOIN users u ON u.id = r.returned_by
	JOIN members m ON m.id = r.member_id
	LEFT JOIN vendor_return_items i ON i.vendor_return_id = r.id`

func (s *Service) ListReturns(ctx context.Context, f DocFilter) ([]Receipt, error) {
	where, args := f.where("r.returned_at", "vendor_return_items", "vendor_return_id")
	rows, err := s.db.Query(ctx, returnSelect+`
		WHERE `+where+`
		GROUP BY r.id, m.name, u.full_name
		ORDER BY r.id DESC LIMIT `+strconv.Itoa(docLimit), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Receipt, error) {
		var r Receipt
		return r, scanReceipt(row, &r)
	})
}

func (s *Service) GetReturn(ctx context.Context, id int64) (*Receipt, error) {
	var r Receipt
	err := scanReceipt(s.db.QueryRow(ctx, returnSelect+`
		WHERE r.id = $1
		GROUP BY r.id, m.name, u.full_name`, id), &r)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReturnNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT p.id, COALESCE(p.barcode, ''), p.name, i.qty, p.price, p.stock
		FROM vendor_return_items i
		JOIN products p ON p.id = i.product_id
		WHERE i.vendor_return_id = $1
		ORDER BY p.name`, id)
	if err != nil {
		return nil, err
	}
	r.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ReceiptLine])
	return &r, err
}

// CreateReturn mengurangi stok dan menulis mutasi 'vendor_return' dalam satu statement.
func (s *Service) CreateReturn(ctx context.Context, userID, memberID int64, note string, lines []ReceiptLine) (int64, error) {
	productIDs := make([]int64, len(lines))
	qtys := make([]int32, len(lines))
	for i, l := range lines {
		productIDs[i], qtys[i] = l.ProductID, l.Qty
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var owned int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM products WHERE id = ANY($1) AND member_id = $2`, productIDs, memberID).Scan(&owned)
	if err != nil {
		return 0, err
	}
	if owned != len(lines) {
		return 0, ErrWrongOwner
	}

	var id int64
	err = tx.QueryRow(ctx, `
		WITH r AS (
			INSERT INTO vendor_returns (member_id, returned_by, note)
			VALUES ($1, $2, NULLIF($3, '')) RETURNING id
		), items AS (
			INSERT INTO vendor_return_items (vendor_return_id, product_id, qty)
			SELECT r.id, x.product_id, x.qty
			FROM r, unnest($4::bigint[], $5::int[]) AS x(product_id, qty)
			RETURNING vendor_return_id, product_id, qty
		), removed AS (
			UPDATE products p SET stock = p.stock - items.qty
			FROM items WHERE p.id = items.product_id
		), moved AS (
			INSERT INTO stock_movements (product_id, qty_change, reason, ref_id, user_id)
			SELECT product_id, -qty, 'vendor_return', vendor_return_id, $2 FROM items
		)
		SELECT id FROM r`, memberID, userID, note, productIDs, qtys).Scan(&id)
	if err != nil {
		return 0, memberError(err)
	}
	return id, tx.Commit(ctx)
}

type StockSummary struct {
	ProductID int64  `json:"product_id"`
	Barcode   string `json:"barcode"`
	Name      string `json:"name"`
	Price     int64  `json:"price"`
	Stock     int    `json:"stock"`
	Received  int    `json:"received"`
	Sold      int    `json:"sold"`
	Returned  int    `json:"returned"`
	Adjusted  int    `json:"adjusted"`
}

// StockSummary merangkum mutasi tiap produk seorang pemilik dalam rentang tanggal (memberID nil = barang toko).
// Stock adalah stok saat ini, bukan stok di akhir rentang. Produk nonaktif ikut tampil selama masih punya stok.
func (s *Service) StockSummary(ctx context.Context, memberID *int64, from, to string) ([]StockSummary, error) {
	rows, err := s.db.Query(ctx, `
		SELECT p.id, COALESCE(p.barcode, ''), p.name, p.price, p.stock,
		       COALESCE(sum(m.qty_change) FILTER (WHERE m.reason = 'receipt'), 0),
		       COALESCE(-sum(m.qty_change) FILTER (WHERE m.reason IN ('sale', 'void')), 0),
		       COALESCE(-sum(m.qty_change) FILTER (WHERE m.reason = 'vendor_return'), 0),
		       COALESCE(sum(m.qty_change) FILTER (WHERE m.reason = 'opname'), 0)
		FROM products p
		LEFT JOIN stock_movements m ON m.product_id = p.id
		     AND m.created_at >= $2::date AND m.created_at < $3::date + 1
		WHERE p.member_id IS NOT DISTINCT FROM $1 AND (p.is_active OR p.stock <> 0)
		GROUP BY p.id
		ORDER BY p.name, p.id`, memberID, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[StockSummary])
}
