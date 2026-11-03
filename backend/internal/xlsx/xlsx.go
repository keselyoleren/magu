// Package xlsx menyusun laporan Excel dengan tampilan seragam: judul, tabel berkepala gelap,
// angka berformat ribuan, baris total, kolom beku, dan saringan otomatis.
package xlsx

import (
	"bytes"
	"time"

	"github.com/xuri/excelize/v2"
)

type Kind int

const (
	Text Kind = iota
	Int
	Money
	Percent
	Date
	DateTime
)

type Column struct {
	Title string
	Width float64
	Kind  Kind
	// Total menjumlahkan kolom ini di baris total.
	Total bool
}

type Book struct {
	file   *excelize.File
	first  bool
	styles map[string]int
}

func New() *Book {
	return &Book{file: excelize.NewFile(), first: true, styles: map[string]int{}}
}

func (b *Book) style(key string, s *excelize.Style) int {
	if id, ok := b.styles[key]; ok {
		return id
	}
	id, _ := b.file.NewStyle(s)
	b.styles[key] = id
	return id
}

var formats = map[Kind]string{
	Int:      `#,##0`,
	Money:    `#,##0`,
	Percent:  `0.##"%"`,
	Date:     `dd mmm yyyy`,
	DateTime: `dd mmm yyyy hh:mm`,
}

func line(color string) []excelize.Border {
	return []excelize.Border{{Type: "bottom", Color: color, Style: 1}}
}

func (b *Book) cellStyle(kind Kind, total bool) int {
	s := &excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: "1C2420"},
		Alignment: &excelize.Alignment{Vertical: "center"},
		Border:    line("E2DCCD"),
	}
	if f, ok := formats[kind]; ok {
		s.CustomNumFmt = &f
	}
	if kind == Int || kind == Money || kind == Percent {
		s.Alignment.Horizontal = "right"
	}
	key := string(rune('a' + kind))
	if total {
		key += "T"
		s.Font.Bold = true
		s.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"F4F1E8"}}
		s.Border = []excelize.Border{{Type: "top", Color: "0F1F1A", Style: 2}}
	}
	return b.style(key, s)
}

// Sheet menambah satu lembar berisi judul, keterangan, dan tabel. Baris total muncul bila ada kolom ber-Total.
// Nilai waktu ditulis sebagai tanggal Excel dalam zona waktu server, jadi bisa disaring dan diurutkan di Excel.
func (b *Book) Sheet(name, title, subtitle string, cols []Column, rows [][]any) {
	f := b.file
	if b.first {
		f.SetSheetName("Sheet1", name)
		b.first = false
	} else {
		f.NewSheet(name)
	}
	f.SetCellValue(name, "A1", title)
	f.SetCellStyle(name, "A1", "A1", b.style("title", &excelize.Style{Font: &excelize.Font{Bold: true, Size: 15, Color: "0F1F1A"}}))
	f.SetCellValue(name, "A2", subtitle)
	f.SetCellStyle(name, "A2", "A2", b.style("subtitle", &excelize.Style{Font: &excelize.Font{Size: 10, Color: "6C736C"}}))
	f.SetRowHeight(name, 1, 24)

	const head = 4
	header := b.style("header", &excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"0F1F1A"}},
		Alignment: &excelize.Alignment{Vertical: "center", WrapText: true},
	})
	headerRight := b.style("headerRight", &excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 10, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"0F1F1A"}},
		Alignment: &excelize.Alignment{Vertical: "center", Horizontal: "right", WrapText: true},
	})
	for i, c := range cols {
		colName, _ := excelize.ColumnNumberToName(i + 1)
		cell, _ := excelize.CoordinatesToCellName(i+1, head)
		f.SetCellValue(name, cell, c.Title)
		style := header
		if c.Kind == Int || c.Kind == Money || c.Kind == Percent {
			style = headerRight
		}
		f.SetCellStyle(name, cell, cell, style)
		f.SetColWidth(name, colName, colName, c.Width)
	}
	f.SetRowHeight(name, head, 22)

	totals := make([]int64, len(cols))
	hasTotal := false
	for r, row := range rows {
		for i, c := range cols {
			cell, _ := excelize.CoordinatesToCellName(i+1, head+1+r)
			v := row[i]
			if t, ok := v.(time.Time); ok {
				// Tanggal kalender dipakai apa adanya; waktu kejadian digeser ke jam toko dulu.
				if c.Kind == Date {
					v = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
				} else {
					t = t.Local()
					v = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
				}
			}
			f.SetCellValue(name, cell, v)
			f.SetCellStyle(name, cell, cell, b.cellStyle(c.Kind, false))
			if c.Total {
				hasTotal = true
				switch n := row[i].(type) {
				case int64:
					totals[i] += n
				case int:
					totals[i] += int64(n)
				}
			}
		}
	}

	end := head + len(rows)
	if hasTotal {
		end++
		for i, c := range cols {
			cell, _ := excelize.CoordinatesToCellName(i+1, end)
			switch {
			case i == 0:
				f.SetCellValue(name, cell, "Total")
			case c.Total:
				f.SetCellValue(name, cell, totals[i])
			}
			kind := c.Kind
			if !c.Total {
				kind = Text
			}
			f.SetCellStyle(name, cell, cell, b.cellStyle(kind, true))
		}
	}

	f.SetPanes(name, &excelize.Panes{Freeze: true, YSplit: head, TopLeftCell: "A5", ActivePane: "bottomLeft"})
	if len(rows) > 0 {
		lastData, _ := excelize.CoordinatesToCellName(len(cols), head+len(rows))
		f.AutoFilter(name, "A4:"+lastData, nil)
	}
	landscape, fit := "landscape", 1
	f.SetPageLayout(name, &excelize.PageLayoutOptions{Orientation: &landscape, FitToWidth: &fit})
	f.SetSheetProps(name, &excelize.SheetPropsOptions{FitToPage: boolPtr(true)})
}

func boolPtr(v bool) *bool { return &v }

func (b *Book) Bytes() ([]byte, error) {
	var buf bytes.Buffer
	if err := b.file.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
