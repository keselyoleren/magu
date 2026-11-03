package catalog

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	snapshotKey = "catalog:snapshot"
	snapshotTTL = 5 * time.Minute
	deltaLimit  = 5000
)

type posItem struct {
	ID      int64  `json:"id"`
	Barcode string `json:"barcode"`
	Name    string `json:"name"`
	Price   int64  `json:"price"`
}

type Change struct {
	ID        int64     `json:"id"`
	Barcode   string    `json:"barcode"`
	Name      string    `json:"name"`
	Price     int64     `json:"price"`
	IsActive  bool      `json:"is_active"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Snapshot mengembalikan seluruh katalog POS sebagai JSON ter-gzip: {"cursor": ..., "items": [...]}.
// Hasilnya disimpan di Redis; snapshot yang agak basi tidak masalah karena klien selalu menyusulkan
// delta sejak cursor-nya.
func (s *Service) Snapshot(ctx context.Context) ([]byte, error) {
	if cached, err := s.cache.Get(ctx, snapshotKey).Bytes(); err == nil {
		return cached, nil
	}

	var cursor time.Time
	if err := s.db.QueryRow(ctx, `SELECT now()`).Scan(&cursor); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(barcode, ''), name, price FROM products WHERE is_active ORDER BY id`)
	if err != nil {
		return nil, err
	}
	items, err := pgx.CollectRows(rows, pgx.RowToStructByPos[posItem])
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(map[string]any{"cursor": cursor, "items": items}); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	s.cache.Set(ctx, snapshotKey, buf.Bytes(), snapshotTTL)
	return buf.Bytes(), nil
}

func (s *Service) ActiveCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM products WHERE is_active`).Scan(&n)
	return n, err
}

// Changes mengembalikan produk yang berubah setelah (since, afterID), terurut, paling banyak deltaLimit baris.
func (s *Service) Changes(ctx context.Context, since time.Time, afterID int64) ([]Change, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, COALESCE(barcode, ''), name, price, is_active, updated_at
		FROM products
		WHERE (updated_at, id) > ($1, $2)
		ORDER BY updated_at, id
		LIMIT $3`, since, afterID, deltaLimit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[Change])
}
