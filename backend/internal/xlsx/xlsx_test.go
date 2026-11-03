package xlsx

import (
	"bytes"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestSheet(t *testing.T) {
	b := New()
	day := time.Date(2026, 10, 12, 14, 30, 0, 0, time.Local)
	b.Sheet("Per hari", "Laporan penjualan", "Toko Uji · 1–12 Okt 2026",
		[]Column{{"Tanggal", 14, Date, false}, {"Transaksi", 11, Int, true}, {"Penjualan", 16, Money, true}, {"Komisi", 10, Percent, false}},
		[][]any{{day, 3, int64(125000), 12.5}, {day, 2, int64(75500), 20.0}})
	b.Sheet("Kosong", "Tanpa data", "", []Column{{"Nama", 20, Text, false}}, nil)

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.GetSheetList(); len(got) != 2 || got[0] != "Per hari" {
		t.Fatalf("sheets = %v", got)
	}
	want := map[string]string{
		"A1": "Laporan penjualan", "A4": "Tanggal", "C4": "Penjualan",
		"A5": "12 Oct 2026", "B5": "3", "C5": "125,000", "D5": "12.5%",
		"A7": "Total", "B7": "5", "C7": "200,500", "D7": "",
	}
	for cell, v := range want {
		if got, _ := f.GetCellValue("Per hari", cell); got != v {
			t.Errorf("%s = %q; want %q", cell, got, v)
		}
	}
	if panes, _ := f.GetPanes("Per hari"); !panes.Freeze || panes.YSplit != 4 {
		t.Errorf("panes = %+v", panes)
	}
}
