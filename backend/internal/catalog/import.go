package catalog

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const maxImportErrors = 20

type importRow struct {
	line          int
	barcode       *string
	name          string
	price         int64
	memberCode    string
	commissionBps *int
}

// parseCSV menerima kolom name, price, dan opsional barcode, member_code, commission_percent, dalam urutan bebas.
// Pemisah `;` dan BOM diterima karena begitulah Excel berbahasa Indonesia mengekspor CSV.
func parseCSV(data []byte) ([]importRow, []string) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	if first, _, _ := bytes.Cut(data, []byte("\n")); bytes.Contains(first, []byte(";")) {
		r.Comma = ';'
	}

	header, err := r.Read()
	if err != nil {
		return nil, []string{"Berkas kosong atau bukan CSV"}
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, required := range []string{"name", "price"} {
		if _, ok := col[required]; !ok {
			return nil, []string{fmt.Sprintf("Kolom %q tidak ada di baris judul", required)}
		}
	}

	var rows []importRow
	var errs []string
	seen := map[string]int{}
	for line := 2; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		fail := func(msg string) {
			if len(errs) < maxImportErrors {
				errs = append(errs, fmt.Sprintf("Baris %d: %s", line, msg))
			}
		}
		if err != nil {
			fail("format CSV rusak")
			continue
		}
		get := func(name string) string {
			i, ok := col[name]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}

		row := importRow{line: line, name: get("name"), memberCode: get("member_code")}
		if row.name == "" {
			fail("nama kosong")
		}
		row.price, err = strconv.ParseInt(get("price"), 10, 64)
		if err != nil || row.price < 0 {
			fail("harga harus angka bulat tanpa titik atau koma")
		}
		if barcode := get("barcode"); barcode != "" {
			if prev, dup := seen[barcode]; dup {
				fail(fmt.Sprintf("barcode %s sama dengan baris %d", barcode, prev))
			}
			seen[barcode] = line
			row.barcode = &barcode
		}
		if percent := get("commission_percent"); percent != "" {
			f, err := strconv.ParseFloat(strings.Replace(percent, ",", ".", 1), 64)
			switch {
			case err != nil || f < 0 || f > 100:
				fail("komisi harus antara 0 dan 100")
			case row.memberCode == "":
				fail("komisi hanya untuk barang titipan (isi member_code)")
			default:
				bps := int(math.Round(f * 100))
				row.commissionBps = &bps
			}
		}
		rows = append(rows, row)
	}
	if len(errs) == 0 && len(rows) == 0 {
		errs = []string{"Tidak ada baris data"}
	}
	return rows, errs
}

// Import menambah produk baru dan memperbarui produk yang barcode-nya sudah ada.
// Baris tanpa barcode selalu menjadi produk baru dengan barcode toko otomatis. Slice kedua berisi pesan kesalahan isi berkas.
func (s *Service) Import(ctx context.Context, data []byte) (int64, []string, error) {
	rows, errs := parseCSV(data)
	if len(errs) > 0 {
		return 0, errs, nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback(ctx)

	memberRows, err := tx.Query(ctx, `SELECT code, id FROM members`)
	if err != nil {
		return 0, nil, err
	}
	members := map[string]int64{}
	var code string
	var id int64
	if _, err := pgx.ForEachRow(memberRows, []any{&code, &id}, func() error {
		members[code] = id
		return nil
	}); err != nil {
		return 0, nil, err
	}

	// Baris tanpa barcode diberi barcode toko, bernomor urut per pemilik seperti produk yang dibuat lewat form.
	nextNumber := map[string]int{}
	records := make([][]any, 0, len(rows))
	for _, row := range rows {
		var memberID *int64
		if row.memberCode != "" {
			id, ok := members[row.memberCode]
			if !ok {
				if len(errs) < maxImportErrors {
					errs = append(errs, fmt.Sprintf("Baris %d: member %s tidak ditemukan", row.line, row.memberCode))
				}
				continue
			}
			memberID = &id
		}
		barcode := row.barcode
		if barcode == nil {
			prefix := s.prefixFor(row.memberCode)
			if _, seen := nextNumber[prefix]; !seen {
				if nextNumber[prefix], err = nextBarcodeNumber(ctx, tx, prefix); err != nil {
					return 0, nil, err
				}
			}
			generated := fmt.Sprintf("%s%05d", prefix, nextNumber[prefix])
			nextNumber[prefix]++
			barcode = &generated
		}
		records = append(records, []any{barcode, row.name, row.price, memberID, row.commissionBps})
	}
	if len(errs) > 0 {
		return 0, errs, nil
	}

	_, err = tx.Exec(ctx, `
		CREATE TEMP TABLE product_import (
			barcode TEXT, name TEXT, price BIGINT, member_id BIGINT, commission_bps INT
		) ON COMMIT DROP`)
	if err != nil {
		return 0, nil, err
	}
	_, err = tx.CopyFrom(ctx, pgx.Identifier{"product_import"},
		[]string{"barcode", "name", "price", "member_id", "commission_bps"}, pgx.CopyFromRows(records))
	if err != nil {
		return 0, nil, err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO products (barcode, name, price, member_id, commission_bps)
		SELECT barcode, name, price, member_id, commission_bps FROM product_import
		ON CONFLICT (barcode) DO UPDATE
		SET name = EXCLUDED.name, price = EXCLUDED.price,
		    member_id = EXCLUDED.member_id, commission_bps = EXCLUDED.commission_bps
		WHERE (products.name, products.price, products.member_id, products.commission_bps)
		      IS DISTINCT FROM (EXCLUDED.name, EXCLUDED.price, EXCLUDED.member_id, EXCLUDED.commission_bps)`)
	if err != nil {
		return 0, nil, err
	}
	return tag.RowsAffected(), nil, tx.Commit(ctx)
}
