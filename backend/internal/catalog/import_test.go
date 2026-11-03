package catalog

import (
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	rows, errs := parseCSV([]byte("\xef\xbb\xbfBarcode;Name;Price;Member_Code;Commission_Percent\r\n" +
		"899001;Keripik;15000;M1;12,5\r\n" +
		";Air Mineral;4000;;\r\n"))
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d", len(rows))
	}
	r := rows[0]
	if *r.barcode != "899001" || r.name != "Keripik" || r.price != 15000 || r.memberCode != "M1" || *r.commissionBps != 1250 {
		t.Errorf("row 0 = %+v", r)
	}
	if rows[1].barcode != nil || rows[1].commissionBps != nil || rows[1].memberCode != "" {
		t.Errorf("row 1 = %+v", rows[1])
	}
}

func TestParseCSVErrors(t *testing.T) {
	cases := map[string]string{
		"":                                   "Berkas kosong",
		"barcode,name\n1,a\n":                `Kolom "price"`,
		"name,price\n":                       "Tidak ada baris data",
		"name,price\n,100\n":                 "Baris 2: nama kosong",
		"name,price\nA,15.000\n":             "Baris 2: harga",
		"name,price,barcode\nA,1,9\nB,2,9\n": "Baris 3: barcode 9 sama dengan baris 2",
		"name,price,commission_percent\nA,1,20\n":                 "Baris 2: komisi hanya untuk barang titipan",
		"name,price,member_code,commission_percent\nA,1,M1,101\n": "Baris 2: komisi harus",
	}
	for input, want := range cases {
		_, errs := parseCSV([]byte(input))
		if len(errs) == 0 || !strings.Contains(errs[0], want) {
			t.Errorf("parseCSV(%q) errors = %v; want %q", input, errs, want)
		}
	}
}
