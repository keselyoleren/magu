package consignment

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"magu/internal/xlsx"
)

func (h *Handler) excelRoutes(r fiber.Router, admin fiber.Handler) {
	r.Get("/reports/sales/xlsx", admin, h.salesExcel)
	r.Get("/reports/stock/xlsx", admin, h.stockExcel)
	r.Get("/settlements/:id/xlsx", admin, h.settlementExcel)
}

func sendExcel(c *fiber.Ctx, book *xlsx.Book, filename string) error {
	data, err := book.Bytes()
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, xlsxContentType)
	c.Attachment(filename)
	return c.Send(data)
}

var months = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func dateText(t time.Time) string {
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()-1], t.Year())
}

func period(from, to time.Time) string {
	if from.Equal(to) {
		return dateText(from)
	}
	return dateText(from) + " – " + dateText(to)
}

var methodNames = map[string]string{"cash": "Tunai", "debit": "Debit", "qris": "QRIS", "cash,debit": "Tunai + Debit", "cash,qris": "Tunai + QRIS", "debit,qris": "Debit + QRIS"}

func (h *Handler) salesExcel(c *fiber.Ctx) error {
	from, errFrom := time.Parse(time.DateOnly, c.Query("from"))
	to, errTo := time.Parse(time.DateOnly, c.Query("to"))
	if errFrom != nil || errTo != nil || to.Before(from) {
		return invalid("Rentang tanggal tidak valid")
	}
	ctx := c.UserContext()
	daily, owners, err := h.svc.SalesReport(ctx, from, to)
	if err != nil {
		return err
	}
	rows, err := h.svc.db.Query(ctx, `
		SELECT t.invoice_no, t.paid_at, u.full_name,
		       COALESCE((SELECT string_agg(DISTINCT method, ',') FROM payments
		                 WHERE transaction_id = t.id AND status = 'paid'), ''),
		       (SELECT sum(qty) FROM transaction_items WHERE transaction_id = t.id)::int, t.total
		FROM transactions t JOIN users u ON u.id = t.cashier_id
		WHERE t.status = 'paid' AND t.paid_at >= $1::date AND t.paid_at < $2::date + 1
		ORDER BY t.paid_at`, from, to)
	if err != nil {
		return err
	}
	type sale struct {
		Invoice string
		PaidAt  time.Time
		Cashier string
		Methods string
		Qty     int
		Total   int64
	}
	sales, err := pgx.CollectRows(rows, pgx.RowToStructByPos[sale])
	if err != nil {
		return err
	}

	sub := h.storeName + " · " + period(from, to)
	book := xlsx.New()

	dayRows := make([][]any, len(daily))
	for i, d := range daily {
		dayRows[i] = []any{d.Day, d.Transactions, d.Cash, d.Debit, d.QRIS, d.Gross, d.StoreShare, d.VendorShare}
	}
	book.Sheet("Per hari", "Laporan penjualan per hari", sub, []xlsx.Column{
		{Title: "Tanggal", Width: 14, Kind: xlsx.Date},
		{Title: "Transaksi", Width: 11, Kind: xlsx.Int, Total: true},
		{Title: "Tunai", Width: 15, Kind: xlsx.Money, Total: true},
		{Title: "Debit", Width: 15, Kind: xlsx.Money, Total: true},
		{Title: "QRIS", Width: 15, Kind: xlsx.Money, Total: true},
		{Title: "Penjualan", Width: 16, Kind: xlsx.Money, Total: true},
		{Title: "Hak toko", Width: 16, Kind: xlsx.Money, Total: true},
		{Title: "Hak penitip", Width: 16, Kind: xlsx.Money, Total: true},
	}, dayRows)

	ownerRows := make([][]any, len(owners))
	for i, o := range owners {
		name := o.Name
		if o.MemberID == nil {
			name = "Barang toko"
		}
		ownerRows[i] = []any{name, o.Qty, o.Gross, o.StoreShare, o.VendorShare}
	}
	book.Sheet("Per pemilik", "Penjualan per pemilik barang", sub, []xlsx.Column{
		{Title: "Pemilik", Width: 30, Kind: xlsx.Text},
		{Title: "Jumlah barang", Width: 14, Kind: xlsx.Int, Total: true},
		{Title: "Penjualan", Width: 16, Kind: xlsx.Money, Total: true},
		{Title: "Hak toko", Width: 16, Kind: xlsx.Money, Total: true},
		{Title: "Hak penitip", Width: 16, Kind: xlsx.Money, Total: true},
	}, ownerRows)

	saleRows := make([][]any, len(sales))
	for i, s := range sales {
		method := methodNames[s.Methods]
		if method == "" {
			method = s.Methods
		}
		saleRows[i] = []any{s.Invoice, s.PaidAt, s.Cashier, method, s.Qty, s.Total}
	}
	book.Sheet("Transaksi", "Daftar transaksi lunas", sub, []xlsx.Column{
		{Title: "No. struk", Width: 20, Kind: xlsx.Text},
		{Title: "Waktu", Width: 19, Kind: xlsx.DateTime},
		{Title: "Kasir", Width: 22, Kind: xlsx.Text},
		{Title: "Pembayaran", Width: 16, Kind: xlsx.Text},
		{Title: "Jumlah barang", Width: 14, Kind: xlsx.Int, Total: true},
		{Title: "Total", Width: 16, Kind: xlsx.Money, Total: true},
	}, saleRows)

	return sendExcel(c, book, fmt.Sprintf("laporan-penjualan-%s-sd-%s.xlsx", from.Format(time.DateOnly), to.Format(time.DateOnly)))
}

func (h *Handler) stockExcel(c *fiber.Ctx) error {
	from, errFrom := time.Parse(time.DateOnly, c.Query("from"))
	to, errTo := time.Parse(time.DateOnly, c.Query("to"))
	if errFrom != nil || errTo != nil {
		return invalid("Rentang tanggal tidak valid")
	}
	var memberID *int64
	if id := int64(c.QueryInt("member_id")); id > 0 {
		memberID = &id
	}
	book, filename, err := h.stockBook(c.UserContext(), memberID, from, to)
	if err != nil {
		return err
	}
	return sendExcel(c, book, filename)
}

func (h *Handler) stockBook(ctx context.Context, memberID *int64, from, to time.Time) (*xlsx.Book, string, error) {
	owner, code := "Barang toko", "toko"
	if memberID != nil {
		err := h.svc.db.QueryRow(ctx, `SELECT name, code FROM members WHERE id = $1`, *memberID).Scan(&owner, &code)
		if err != nil {
			return nil, "", fiber.NewError(fiber.StatusNotFound, ErrMemberNotFound.Error())
		}
	}
	summary, err := h.svc.StockSummary(ctx, memberID, from.Format(time.DateOnly), to.Format(time.DateOnly))
	if err != nil {
		return nil, "", err
	}
	rows := make([][]any, len(summary))
	for i, r := range summary {
		rows[i] = []any{r.Barcode, r.Name, r.Price, r.Received, r.Sold, r.Returned, r.Adjusted, r.Stock, int64(max(r.Stock, 0)) * r.Price}
	}
	book := xlsx.New()
	book.Sheet("Stok", "Stok "+owner, h.storeName+" · mutasi "+period(from, to)+" · sisa stok per "+dateText(time.Now()),
		[]xlsx.Column{
			{Title: "Barcode", Width: 16, Kind: xlsx.Text},
			{Title: "Produk", Width: 38, Kind: xlsx.Text},
			{Title: "Harga", Width: 13, Kind: xlsx.Money},
			{Title: "Masuk", Width: 10, Kind: xlsx.Int, Total: true},
			{Title: "Terjual", Width: 10, Kind: xlsx.Int, Total: true},
			{Title: "Retur", Width: 10, Kind: xlsx.Int, Total: true},
			{Title: "Selisih opname", Width: 12, Kind: xlsx.Int, Total: true},
			{Title: "Sisa stok", Width: 11, Kind: xlsx.Int, Total: true},
			{Title: "Nilai sisa", Width: 16, Kind: xlsx.Money, Total: true},
		}, rows)
	return book, fmt.Sprintf("stok-%s-%s.xlsx", code, to.Format(time.DateOnly)), nil
}

func (h *Handler) settlementExcel(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	st, err := h.svc.GetSettlement(c.UserContext(), id)
	if err != nil {
		return settlementError(err)
	}
	book, filename := h.settlementBook(st)
	return sendExcel(c, book, filename)
}

func (h *Handler) settlementBook(st *Settlement) (*xlsx.Book, string) {
	status := "belum dibayar"
	if st.PaidAt != nil {
		status = "dibayar " + dateText(st.PaidAt.Local())
	}
	sub := fmt.Sprintf("%s · %s (%s) · %s · %s", h.storeName, st.MemberName, st.MemberCode, period(st.PeriodStart, st.PeriodEnd), status)
	if st.Bank != "" {
		sub += " · " + st.Bank
	}
	rows := make([][]any, len(st.Lines))
	for i, l := range st.Lines {
		rows[i] = []any{l.PaidAt, l.InvoiceNo, l.ProductName, l.Qty, l.LineTotal, float64(l.CommissionBps) / 100, l.CommissionAmount, l.VendorAmount}
	}
	book := xlsx.New()
	book.Sheet("Bagi hasil", fmt.Sprintf("Pembagian hasil #%d", st.ID), sub, []xlsx.Column{
		{Title: "Tanggal", Width: 19, Kind: xlsx.DateTime},
		{Title: "No. struk", Width: 20, Kind: xlsx.Text},
		{Title: "Produk", Width: 36, Kind: xlsx.Text},
		{Title: "Jumlah", Width: 9, Kind: xlsx.Int, Total: true},
		{Title: "Penjualan", Width: 15, Kind: xlsx.Money, Total: true},
		{Title: "Komisi toko", Width: 11, Kind: xlsx.Percent},
		{Title: "Hak toko", Width: 15, Kind: xlsx.Money, Total: true},
		{Title: "Hak penitip", Width: 15, Kind: xlsx.Money, Total: true},
	}, rows)
	return book, fmt.Sprintf("bagi-hasil-%d-%s.xlsx", st.ID, st.MemberCode)
}
