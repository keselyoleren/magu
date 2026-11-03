package catalog

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"magu/internal/pgerr"
)

var (
	ErrProductNotFound = errors.New("produk tidak ditemukan")
	ErrBarcodeTaken    = errors.New("barcode sudah dipakai produk lain")
	ErrMemberNotFound  = errors.New("member tidak ditemukan")
	ErrProductInUse    = errors.New("produk sudah punya riwayat stok atau penjualan, jadi tidak bisa dihapus; nonaktifkan saja")
)

const (
	pageSize    = 50
	maxPageSize = 1000
)

// StoreOwned sebagai memberID di List berarti "hanya barang milik toko".
const StoreOwned = -1

type Service struct {
	db        *pgxpool.Pool
	cache     *redis.Client
	storeCode string
}

// storeCode menjadi awalan barcode untuk barang milik toko.
func NewService(db *pgxpool.Pool, cache *redis.Client, storeCode string) *Service {
	return &Service{db: db, cache: cache, storeCode: storeCode}
}

type Product struct {
	ID            int64  `json:"id"`
	MemberID      *int64 `json:"member_id"`
	MemberName    string `json:"member_name"`
	Barcode       string `json:"barcode"`
	Name          string `json:"name"`
	Price         int64  `json:"price"`
	CommissionBps *int   `json:"commission_bps"`
	Stock         int    `json:"stock"`
	IsActive      bool   `json:"is_active"`
}

// List menyusun WHERE dari potongan tetap, bukan `$1 = ” OR ...`, supaya planner tetap bisa
// memakai index trigram dan index barcode.
// perPage 0 berarti ukuran halaman bawaan.
func (s *Service) List(ctx context.Context, search string, memberID int64, page, perPage int) ([]Product, bool, error) {
	if perPage <= 0 {
		perPage = pageSize
	}
	perPage = min(perPage, maxPageSize)
	where := "TRUE"
	var args []any
	if search != "" {
		args = append(args, search)
		where += fmt.Sprintf(" AND (p.name ILIKE '%%' || $%d || '%%' OR p.barcode IN ($%d, upper($%d)))", len(args), len(args), len(args))
	}
	switch {
	case memberID == StoreOwned:
		where += " AND p.member_id IS NULL"
	case memberID != 0:
		args = append(args, memberID)
		where += fmt.Sprintf(" AND p.member_id = $%d", len(args))
	}
	args = append(args, perPage+1, page*perPage)

	rows, err := s.db.Query(ctx, fmt.Sprintf(`
		SELECT p.id, p.member_id, COALESCE(m.name, ''), COALESCE(p.barcode, ''), p.name, p.price,
		       p.commission_bps, p.stock, p.is_active
		FROM products p
		LEFT JOIN members m ON m.id = p.member_id
		WHERE %s
		ORDER BY p.name, p.id
		LIMIT $%d OFFSET $%d`, where, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	products, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Product])
	if err != nil {
		return nil, false, err
	}
	if len(products) > perPage {
		return products[:perPage], true, nil
	}
	return products, false, nil
}

func (s *Service) Get(ctx context.Context, id int64) (*Product, error) {
	rows, err := s.db.Query(ctx, `
		SELECT p.id, p.member_id, COALESCE(m.name, ''), COALESCE(p.barcode, ''), p.name, p.price,
		       p.commission_bps, p.stock, p.is_active
		FROM products p
		LEFT JOIN members m ON m.id = p.member_id
		WHERE p.id = $1`, id)
	if err != nil {
		return nil, err
	}
	p, err := pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByPos[Product])
	return p, productError(err)
}

// NextBarcode membuat barcode toko: kode pemilik (kode member, atau kode toko untuk barang milik toko)
// diikuti lima digit nomor urut per pemilik, mis. "KM00012". Hanya huruf besar dan angka yang dipakai,
// supaya terbaca sama oleh scanner apa pun.
func (s *Service) NextBarcode(ctx context.Context, memberID *int64) (string, error) {
	prefix := s.storeCode
	if memberID != nil {
		err := s.db.QueryRow(ctx, `SELECT code FROM members WHERE id = $1`, *memberID).Scan(&prefix)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrMemberNotFound
		}
		if err != nil {
			return "", err
		}
	}
	prefix = s.prefixFor(prefix)
	next, err := nextBarcodeNumber(ctx, s.db, prefix)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s%05d", prefix, next), nil
}

// prefixFor membersihkan kode pemilik; kode yang tidak menyisakan huruf atau angka diganti kode toko.
func (s *Service) prefixFor(code string) string {
	if prefix := barcodePrefix(code); prefix != "" {
		return prefix
	}
	return barcodePrefix(s.storeCode)
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

func nextBarcodeNumber(ctx context.Context, q querier, prefix string) (int, error) {
	var next int
	err := q.QueryRow(ctx, `
		SELECT COALESCE(max(right(barcode, 5)::int), 0) + 1
		FROM products WHERE barcode ~ ('^' || $1 || '[0-9]{5}$')`, prefix).Scan(&next)
	return next, err
}

func barcodePrefix(code string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(code) {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Create mengisi barcode otomatis bila dikosongkan.
func (s *Service) Create(ctx context.Context, p *Product) error {
	if p.Barcode != "" {
		return s.insert(ctx, p)
	}
	// Dua form yang disimpan bersamaan bisa mendapat nomor yang sama; yang kalah mencoba nomor berikutnya.
	for attempt := 0; ; attempt++ {
		barcode, err := s.NextBarcode(ctx, p.MemberID)
		if err != nil {
			return err
		}
		p.Barcode = barcode
		if err = s.insert(ctx, p); !errors.Is(err, ErrBarcodeTaken) || attempt == 3 {
			return err
		}
	}
}

func (s *Service) insert(ctx context.Context, p *Product) error {
	err := s.db.QueryRow(ctx, `
		INSERT INTO products (member_id, barcode, name, price, commission_bps, is_active)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6)
		RETURNING id`,
		p.MemberID, p.Barcode, p.Name, p.Price, p.CommissionBps, p.IsActive).Scan(&p.ID)
	return productError(err)
}

func (s *Service) Update(ctx context.Context, p *Product) error {
	err := s.db.QueryRow(ctx, `
		UPDATE products
		SET member_id = $2, barcode = NULLIF($3, ''), name = $4, price = $5, commission_bps = $6, is_active = $7
		WHERE id = $1
		RETURNING stock`,
		p.ID, p.MemberID, p.Barcode, p.Name, p.Price, p.CommissionBps, p.IsActive).Scan(&p.Stock)
	return productError(err)
}

type PriceChange struct {
	ID       int64 `json:"id"`
	Price    int64 `json:"price"`
	IsActive bool  `json:"is_active"`
}

// SetPrices mengubah harga dan status aktif banyak produk sekaligus; mengembalikan jumlah produk yang berubah.
func (s *Service) SetPrices(ctx context.Context, changes []PriceChange) (int64, error) {
	ids := make([]int64, len(changes))
	prices := make([]int64, len(changes))
	active := make([]bool, len(changes))
	for i, c := range changes {
		ids[i], prices[i], active[i] = c.ID, c.Price, c.IsActive
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE products p SET price = x.price, is_active = x.is_active
		FROM unnest($1::bigint[], $2::bigint[], $3::boolean[]) AS x(id, price, is_active)
		WHERE p.id = x.id AND (p.price, p.is_active) IS DISTINCT FROM (x.price, x.is_active)`, ids, prices, active)
	return tag.RowsAffected(), err
}

// Delete hanya berhasil untuk produk tanpa riwayat; foreign key dari dokumen stok dan penjualan yang menolak sisanya.
func (s *Service) Delete(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM products WHERE id = $1`, id)
	if pgerr.IsForeignKey(err) {
		return ErrProductInUse
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrProductNotFound
	}
	// Snapshot lama masih memuat produk ini, dan delta tidak bisa membuangnya dari terminal kasir.
	if s.cache != nil {
		s.cache.Del(ctx, snapshotKey)
	}
	return nil
}

func productError(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ErrProductNotFound
	case pgerr.IsUnique(err):
		return ErrBarcodeTaken
	case pgerr.IsForeignKey(err):
		return ErrMemberNotFound
	}
	return err
}
