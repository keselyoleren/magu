package consignment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"magu/internal/catalog"
)

// Layar riwayat menampilkan paling banyak sekian dokumen; lebih dari itu pengguna mempersempit saringan.
const docLimit = 200

var (
	ErrReceiptNotFound  = errors.New("penerimaan tidak ditemukan")
	ErrReceiptCancelled = errors.New("penerimaan ini sudah dibatalkan")
	ErrWrongOwner       = errors.New("ada produk yang bukan milik pemilik yang dipilih")
)

type ReceiptLine struct {
	ProductID int64  `json:"product_id"`
	Barcode   string `json:"barcode"`
	Name      string `json:"name"`
	Qty       int32  `json:"qty"`
	Price     int64  `json:"price"`
	Stock     int32  `json:"stock"`
}

type Receipt struct {
	ID          int64         `json:"id"`
	MemberID    *int64        `json:"member_id"`
	MemberName  string        `json:"member_name"`
	Note        string        `json:"note"`
	ReceivedAt  time.Time     `json:"received_at"`
	ReceivedBy  string        `json:"received_by"`
	ItemCount   int           `json:"item_count"`
	TotalQty    int           `json:"total_qty"`
	CancelledAt *time.Time    `json:"cancelled_at"`
	CancelledBy string        `json:"cancelled_by"`
	Items       []ReceiptLine `json:"items,omitempty"`
}

// DocFilter menyaring riwayat penerimaan dan retur. MemberID 0 = semua, catalog.StoreOwned = barang toko.
// From dan To berupa tanggal 'YYYY-MM-DD', kosong berarti tanpa batas.
type DocFilter struct {
	MemberID int64
	From     string
	To       string
	Search   string
}

// where memakai alias r untuk dokumen. items adalah tabel barisnya dengan alias x, link kolom x yang menunjuk ke r.
func (f DocFilter) where(dateColumn, items, link string) (string, []any) {
	where := "TRUE"
	var args []any
	switch {
	case f.MemberID == catalog.StoreOwned:
		where += " AND r.member_id IS NULL"
	case f.MemberID != 0:
		args = append(args, f.MemberID)
		where += fmt.Sprintf(" AND r.member_id = $%d", len(args))
	}
	if f.From != "" {
		args = append(args, f.From)
		where += fmt.Sprintf(" AND %s >= $%d::date", dateColumn, len(args))
	}
	if f.To != "" {
		args = append(args, f.To)
		where += fmt.Sprintf(" AND %s < $%d::date + 1", dateColumn, len(args))
	}
	if f.Search != "" {
		args = append(args, f.Search)
		n := len(args)
		where += fmt.Sprintf(` AND (r.id::text = ltrim($%d, '#') OR r.note ILIKE '%%' || $%d || '%%' OR EXISTS (
			SELECT 1 FROM %s x JOIN products p ON p.id = x.product_id
			WHERE x.%s = r.id AND (p.name ILIKE '%%' || $%d || '%%' OR p.barcode IN ($%d, upper($%d)))))`,
			n, n, items, link, n, n, n)
	}
	return where, args
}

const receiptSelect = `
	SELECT r.id, r.member_id, COALESCE(m.name, ''), COALESCE(r.note, ''), r.received_at, u.full_name,
	       count(i.id), COALESCE(sum(i.qty), 0), r.cancelled_at, COALESCE(c.full_name, '')
	FROM goods_receipts r
	JOIN users u ON u.id = r.received_by
	LEFT JOIN users c ON c.id = r.cancelled_by
	LEFT JOIN members m ON m.id = r.member_id
	LEFT JOIN goods_receipt_items i ON i.goods_receipt_id = r.id`

func scanReceipt(row pgx.Row, r *Receipt) error {
	return row.Scan(&r.ID, &r.MemberID, &r.MemberName, &r.Note, &r.ReceivedAt, &r.ReceivedBy, &r.ItemCount, &r.TotalQty,
		&r.CancelledAt, &r.CancelledBy)
}

func (s *Service) ListReceipts(ctx context.Context, f DocFilter) ([]Receipt, error) {
	where, args := f.where("r.received_at", "goods_receipt_items", "goods_receipt_id")
	rows, err := s.db.Query(ctx, receiptSelect+`
		WHERE `+where+`
		GROUP BY r.id, m.name, u.full_name, c.full_name
		ORDER BY r.id DESC LIMIT `+strconv.Itoa(docLimit), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Receipt, error) {
		var r Receipt
		return r, scanReceipt(row, &r)
	})
}

func (s *Service) GetReceipt(ctx context.Context, id int64) (*Receipt, error) {
	var r Receipt
	err := scanReceipt(s.db.QueryRow(ctx, receiptSelect+`
		WHERE r.id = $1
		GROUP BY r.id, m.name, u.full_name, c.full_name`, id), &r)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrReceiptNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT p.id, COALESCE(p.barcode, ''), p.name, i.qty, p.price, p.stock
		FROM goods_receipt_items i
		JOIN products p ON p.id = i.product_id
		WHERE i.goods_receipt_id = $1
		ORDER BY p.name`, id)
	if err != nil {
		return nil, err
	}
	r.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ReceiptLine])
	return &r, err
}

// CreateReceipt menambah stok dan menulis mutasinya dalam satu statement.
func (s *Service) CreateReceipt(ctx context.Context, userID int64, memberID *int64, note string, lines []ReceiptLine) (int64, error) {
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
	err = tx.QueryRow(ctx, `
		SELECT count(*) FROM products
		WHERE id = ANY($1) AND member_id IS NOT DISTINCT FROM $2`, productIDs, memberID).Scan(&owned)
	if err != nil {
		return 0, err
	}
	if owned != len(lines) {
		return 0, ErrWrongOwner
	}

	var id int64
	err = tx.QueryRow(ctx, `
		WITH r AS (
			INSERT INTO goods_receipts (member_id, received_by, note)
			VALUES ($1, $2, NULLIF($3, '')) RETURNING id
		), items AS (
			INSERT INTO goods_receipt_items (goods_receipt_id, product_id, qty)
			SELECT r.id, x.product_id, x.qty
			FROM r, unnest($4::bigint[], $5::int[]) AS x(product_id, qty)
			RETURNING goods_receipt_id, product_id, qty
		), added AS (
			UPDATE products p SET stock = p.stock + items.qty
			FROM items WHERE p.id = items.product_id
		), moved AS (
			INSERT INTO stock_movements (product_id, qty_change, reason, ref_id, user_id)
			SELECT product_id, qty, 'receipt', goods_receipt_id, $2 FROM items
		)
		SELECT id FROM r`, memberID, userID, note, productIDs, qtys).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

// CancelReceipt menarik kembali stok lewat mutasi 'receipt' negatif, supaya jumlah diterima di Stok titipan ikut turun.
// Stok boleh menjadi minus kalau barangnya sudah telanjur terjual. Syarat cancelled_at IS NULL di UPDATE
// mencegah stok ditarik dua kali saat dua admin membatalkan bersamaan.
func (s *Service) CancelReceipt(ctx context.Context, userID, id int64) error {
	var cancelled int
	err := s.db.QueryRow(ctx, `
		WITH r AS (
			UPDATE goods_receipts SET cancelled_at = now(), cancelled_by = $2
			WHERE id = $1 AND cancelled_at IS NULL RETURNING id
		), items AS (
			SELECT i.goods_receipt_id, i.product_id, i.qty
			FROM goods_receipt_items i JOIN r ON r.id = i.goods_receipt_id
		), removed AS (
			UPDATE products p SET stock = p.stock - items.qty
			FROM items WHERE p.id = items.product_id
		), moved AS (
			INSERT INTO stock_movements (product_id, qty_change, reason, ref_id, user_id)
			SELECT product_id, -qty, 'receipt', goods_receipt_id, $2 FROM items
		)
		SELECT count(*) FROM r`, id, userID).Scan(&cancelled)
	if err != nil {
		return err
	}
	if cancelled == 1 {
		return nil
	}
	if _, err := s.GetReceipt(ctx, id); err != nil {
		return err
	}
	return ErrReceiptCancelled
}
