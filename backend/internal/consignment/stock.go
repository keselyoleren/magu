package consignment

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrProductNotFound = errors.New("produk tidak ditemukan")

type Movement struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Reason    string    `json:"reason"`
	QtyChange int       `json:"qty_change"`
	RefID     *int64    `json:"ref_id"`
	User      string    `json:"user"`
	Note      string    `json:"note"`
}

type StockCard struct {
	ProductID int64      `json:"product_id"`
	Name      string     `json:"name"`
	Stock     int        `json:"stock"`
	Movements []Movement `json:"movements"`
}

func (s *Service) StockCard(ctx context.Context, productID int64) (*StockCard, error) {
	card := StockCard{ProductID: productID}
	err := s.db.QueryRow(ctx, `SELECT name, stock FROM products WHERE id = $1`, productID).Scan(&card.Name, &card.Stock)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT sm.id, sm.created_at, sm.reason, sm.qty_change, sm.ref_id, COALESCE(u.full_name, ''),
		       COALESCE(o.note, CASE WHEN sm.reason = 'receipt' AND sm.qty_change < 0 THEN 'Penerimaan dibatalkan' END, '')
		FROM stock_movements sm
		LEFT JOIN users u ON u.id = sm.user_id
		LEFT JOIN stock_opnames o ON sm.reason = 'opname' AND o.id = sm.ref_id
		WHERE sm.product_id = $1
		ORDER BY sm.id DESC LIMIT 200`, productID)
	if err != nil {
		return nil, err
	}
	card.Movements, err = pgx.CollectRows(rows, pgx.RowToStructByPos[Movement])
	return &card, err
}
