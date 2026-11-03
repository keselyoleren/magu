package consignment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrOpnameNotFound = errors.New("stok opname tidak ditemukan")
	ErrOpnamePosted   = errors.New("stok opname sudah diposting")
	ErrNoProducts     = errors.New("pemilik ini belum punya produk aktif")
	ErrNothingCounted = errors.New("belum ada produk yang dihitung")
)

type OpnameItem struct {
	ProductID   int64  `json:"product_id"`
	Barcode     string `json:"barcode"`
	Name        string `json:"name"`
	SystemQty   int    `json:"system_qty"`
	PhysicalQty *int   `json:"physical_qty"`
}

type Opname struct {
	ID         int64        `json:"id"`
	MemberID   *int64       `json:"member_id"`
	MemberName string       `json:"member_name"`
	Status     string       `json:"status"`
	Note       string       `json:"note"`
	CreatedAt  time.Time    `json:"created_at"`
	PostedAt   *time.Time   `json:"posted_at"`
	ItemCount  int          `json:"item_count"`
	Counted    int          `json:"counted"`
	DiffCount  int          `json:"diff_count"`
	Items      []OpnameItem `json:"items,omitempty"`
}

const opnameSelect = `
	SELECT o.id, o.member_id, COALESCE(m.name, ''), o.status, COALESCE(o.note, ''), o.created_at, o.posted_at,
	       count(i.id), count(i.physical_qty), count(*) FILTER (WHERE i.diff <> 0)
	FROM stock_opnames o
	LEFT JOIN members m ON m.id = o.member_id
	LEFT JOIN stock_opname_items i ON i.stock_opname_id = o.id`

func scanOpname(row pgx.Row, o *Opname) error {
	return row.Scan(&o.ID, &o.MemberID, &o.MemberName, &o.Status, &o.Note, &o.CreatedAt, &o.PostedAt,
		&o.ItemCount, &o.Counted, &o.DiffCount)
}

func (s *Service) ListOpnames(ctx context.Context) ([]Opname, error) {
	rows, err := s.db.Query(ctx, opnameSelect+`
		GROUP BY o.id, m.name
		ORDER BY o.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Opname, error) {
		var o Opname
		return o, scanOpname(row, &o)
	})
}

func (s *Service) GetOpname(ctx context.Context, id int64) (*Opname, error) {
	var o Opname
	err := scanOpname(s.db.QueryRow(ctx, opnameSelect+`
		WHERE o.id = $1
		GROUP BY o.id, m.name`, id), &o)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrOpnameNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT p.id, COALESCE(p.barcode, ''), p.name, i.system_qty, i.physical_qty
		FROM stock_opname_items i
		JOIN products p ON p.id = i.product_id
		WHERE i.stock_opname_id = $1
		ORDER BY p.name, p.id`, id)
	if err != nil {
		return nil, err
	}
	o.Items, err = pgx.CollectRows(rows, pgx.RowToStructByPos[OpnameItem])
	return &o, err
}

// CreateOpname membuat draft berisi semua produk aktif milik satu pemilik (memberID nil = barang toko).
func (s *Service) CreateOpname(ctx context.Context, userID int64, memberID *int64, note string) (int64, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var id int64
	err = tx.QueryRow(ctx, `
		INSERT INTO stock_opnames (member_id, note, created_by)
		VALUES ($1, NULLIF($2, ''), $3) RETURNING id`, memberID, note, userID).Scan(&id)
	if err != nil {
		return 0, memberError(err)
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO stock_opname_items (stock_opname_id, product_id, system_qty)
		SELECT $1, id, stock FROM products
		WHERE is_active AND member_id IS NOT DISTINCT FROM $2`, id, memberID)
	if err != nil {
		return 0, err
	}
	if tag.RowsAffected() == 0 {
		return 0, ErrNoProducts
	}
	return id, tx.Commit(ctx)
}

func (s *Service) SetPhysicalQty(ctx context.Context, opnameID, productID int64, qty *int) error {
	tag, err := s.db.Exec(ctx, `
		UPDATE stock_opname_items i SET physical_qty = $3
		FROM stock_opnames o
		WHERE o.id = i.stock_opname_id AND o.status = 'draft'
		  AND i.stock_opname_id = $1 AND i.product_id = $2`, opnameID, productID, qty)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.draftError(ctx, opnameID)
	}
	return nil
}

func (s *Service) DeleteOpname(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM stock_opnames WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.draftError(ctx, id)
	}
	return nil
}

// PostOpname membaca ulang dan mengunci stok saat ini, bukan memakai angka waktu draft dibuat,
// karena stok bisa berubah selama penghitungan. Produk yang belum dihitung tidak disentuh.
func (s *Service) PostOpname(ctx context.Context, id, userID int64) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := s.postOpname(ctx, tx, id, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// StockChange adalah jumlah stok yang benar untuk satu produk.
type StockChange struct {
	ID    int64  `json:"id"`
	Stock *int32 `json:"stock"`
}

// AdjustStocks mengoreksi stok tanpa membuat lembar hitung. Bentuknya opname yang langsung diposting, satu dokumen
// per pemilik karena opname selalu milik satu pemilik, supaya koreksi tetap punya dokumen, catatan, dan mutasi yang
// sama dengan opname biasa. Semua atau tidak sama sekali.
func (s *Service) AdjustStocks(ctx context.Context, userID int64, changes []StockChange, note string) error {
	text := "Koreksi stok"
	if note != "" {
		text += ": " + note
	}
	ids := make([]int64, len(changes))
	stocks := make([]int32, len(changes))
	for i, c := range changes {
		ids[i], stocks[i] = c.ID, *c.Stock
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `SELECT DISTINCT member_id FROM products WHERE id = ANY($1)`, ids)
	if err != nil {
		return err
	}
	owners, err := pgx.CollectRows(rows, pgx.RowTo[*int64])
	if err != nil {
		return err
	}

	found := 0
	for _, owner := range owners {
		var id int64
		var count int
		err = tx.QueryRow(ctx, `
			WITH o AS (
				INSERT INTO stock_opnames (member_id, note, created_by)
				VALUES ($1::bigint, $2, $3) RETURNING id
			), item AS (
				INSERT INTO stock_opname_items (stock_opname_id, product_id, system_qty, physical_qty)
				SELECT o.id, p.id, p.stock, x.stock
				FROM o, unnest($4::bigint[], $5::int[]) AS x(id, stock)
				JOIN products p ON p.id = x.id
				WHERE p.member_id IS NOT DISTINCT FROM $1::bigint
				RETURNING 1
			)
			SELECT o.id, (SELECT count(*) FROM item) FROM o`, owner, text, userID, ids, stocks).Scan(&id, &count)
		if err != nil {
			return err
		}
		if err := s.postOpname(ctx, tx, id, userID); err != nil {
			return err
		}
		found += count
	}
	if found != len(changes) {
		return ErrProductNotFound
	}
	return tx.Commit(ctx)
}

func (s *Service) postOpname(ctx context.Context, tx pgx.Tx, id, userID int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE stock_opnames SET status = 'posted', posted_at = now()
		WHERE id = $1 AND status = 'draft'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.draftError(ctx, id)
	}

	var counted int
	err = tx.QueryRow(ctx, `
		WITH cur AS (
			SELECT p.id, p.stock
			FROM products p
			JOIN stock_opname_items i ON i.product_id = p.id
			WHERE i.stock_opname_id = $1 AND i.physical_qty IS NOT NULL
			FOR UPDATE OF p
		), snap AS (
			UPDATE stock_opname_items i SET system_qty = cur.stock
			FROM cur
			WHERE i.stock_opname_id = $1 AND i.product_id = cur.id
			RETURNING i.product_id, i.physical_qty, i.physical_qty - cur.stock AS diff
		), adjusted AS (
			UPDATE products p SET stock = snap.physical_qty
			FROM snap WHERE p.id = snap.product_id AND snap.diff <> 0
		), moved AS (
			INSERT INTO stock_movements (product_id, qty_change, reason, ref_id, user_id)
			SELECT product_id, diff, 'opname', $1, $2 FROM snap WHERE diff <> 0
		)
		SELECT count(*) FROM snap`, id, userID).Scan(&counted)
	if err != nil {
		return err
	}
	if counted == 0 {
		return ErrNothingCounted
	}
	return nil
}

func (s *Service) draftError(ctx context.Context, id int64) error {
	var status string
	err := s.db.QueryRow(ctx, `SELECT status FROM stock_opnames WHERE id = $1`, id).Scan(&status)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrOpnameNotFound
	case err != nil:
		return err
	case status == "posted":
		return ErrOpnamePosted
	}
	return ErrOpnameNotFound
}
