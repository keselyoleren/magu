package consignment

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"magu/internal/auth"
)

func (h *Handler) stockRoutes(r fiber.Router, admin fiber.Handler) {
	r.Get("/receipts", admin, h.listReceipts)
	r.Post("/receipts", admin, h.createReceipt)
	r.Get("/receipts/:id", admin, h.getReceipt)
	r.Post("/receipts/:id/cancel", admin, h.cancelReceipt)

	r.Get("/opnames", admin, h.listOpnames)
	r.Post("/opnames", admin, h.createOpname)
	r.Get("/opnames/:id", admin, h.getOpname)
	r.Delete("/opnames/:id", admin, h.deleteOpname)
	r.Post("/opnames/:id/post", admin, h.postOpname)
	r.Put("/opnames/:id/items/:product_id", admin, h.setPhysicalQty)

	r.Get("/returns", admin, h.listReturns)
	r.Post("/returns", admin, h.createReturn)
	r.Get("/returns/:id", admin, h.getReturn)

	r.Get("/products/:id/movements", admin, h.stockCard)
	r.Post("/products/stock", admin, h.adjustStocks)
	r.Get("/reports/stock", admin, h.stockSummary)
}

func (h *Handler) adjustStocks(c *fiber.Ctx) error {
	var in struct {
		Note  string        `json:"note"`
		Items []StockChange `json:"items"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	if len(in.Items) == 0 {
		return invalid("Belum ada stok yang diubah")
	}
	seen := map[int64]bool{}
	for _, item := range in.Items {
		if item.Stock == nil || *item.Stock < 0 {
			return invalid("Jumlah stok tidak boleh kosong atau minus")
		}
		if seen[item.ID] {
			return invalid("Ada produk yang tercantum dua kali")
		}
		seen[item.ID] = true
	}
	if err := h.svc.AdjustStocks(c.UserContext(), auth.CurrentUser(c).ID, in.Items, strings.TrimSpace(in.Note)); err != nil {
		return stockError(err)
	}
	return c.JSON(fiber.Map{"updated": len(in.Items)})
}

func docFilter(c *fiber.Ctx) (DocFilter, error) {
	f := DocFilter{
		MemberID: int64(c.QueryInt("member_id")),
		From:     c.Query("from"),
		To:       c.Query("to"),
		Search:   strings.TrimSpace(c.Query("q")),
	}
	for _, day := range []string{f.From, f.To} {
		if _, err := time.Parse(time.DateOnly, day); day != "" && err != nil {
			return f, invalid("Rentang tanggal tidak valid")
		}
	}
	return f, nil
}

func (h *Handler) listReturns(c *fiber.Ctx) error {
	f, err := docFilter(c)
	if err != nil {
		return err
	}
	returns, err := h.svc.ListReturns(c.UserContext(), f)
	if err != nil {
		return err
	}
	return c.JSON(returns)
}

func (h *Handler) getReturn(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	ret, err := h.svc.GetReturn(c.UserContext(), id)
	if err != nil {
		return stockError(err)
	}
	return c.JSON(ret)
}

func (h *Handler) createReturn(c *fiber.Ctx) error {
	var in struct {
		MemberID *int64        `json:"member_id"`
		Note     string        `json:"note"`
		Items    []ReceiptLine `json:"items"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	if in.MemberID == nil {
		return invalid("Retur hanya untuk barang titipan; pilih penitipnya")
	}
	if err := validLines(in.Items); err != nil {
		return err
	}
	id, err := h.svc.CreateReturn(c.UserContext(), auth.CurrentUser(c).ID, *in.MemberID, strings.TrimSpace(in.Note), in.Items)
	if err != nil {
		return stockError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (h *Handler) stockSummary(c *fiber.Ctx) error {
	from, to := c.Query("from"), c.Query("to")
	_, errFrom := time.Parse(time.DateOnly, from)
	_, errTo := time.Parse(time.DateOnly, to)
	if errFrom != nil || errTo != nil {
		return invalid("Rentang tanggal tidak valid")
	}
	var memberID *int64
	if id := int64(c.QueryInt("member_id")); id > 0 {
		memberID = &id
	}
	rows, err := h.svc.StockSummary(c.UserContext(), memberID, from, to)
	if err != nil {
		return err
	}
	return c.JSON(rows)
}

func validLines(items []ReceiptLine) error {
	if len(items) == 0 {
		return invalid("Belum ada produk")
	}
	seen := map[int64]bool{}
	for _, item := range items {
		if item.Qty <= 0 {
			return invalid("Jumlah harus lebih dari 0")
		}
		if seen[item.ProductID] {
			return invalid("Ada produk yang tercantum dua kali")
		}
		seen[item.ProductID] = true
	}
	return nil
}

func (h *Handler) listReceipts(c *fiber.Ctx) error {
	f, err := docFilter(c)
	if err != nil {
		return err
	}
	receipts, err := h.svc.ListReceipts(c.UserContext(), f)
	if err != nil {
		return err
	}
	return c.JSON(receipts)
}

func (h *Handler) cancelReceipt(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.CancelReceipt(c.UserContext(), auth.CurrentUser(c).ID, id); err != nil {
		return stockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) getReceipt(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	receipt, err := h.svc.GetReceipt(c.UserContext(), id)
	if err != nil {
		return stockError(err)
	}
	return c.JSON(receipt)
}

func (h *Handler) createReceipt(c *fiber.Ctx) error {
	var in struct {
		MemberID *int64        `json:"member_id"`
		Note     string        `json:"note"`
		Items    []ReceiptLine `json:"items"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	if err := validLines(in.Items); err != nil {
		return err
	}

	id, err := h.svc.CreateReceipt(c.UserContext(), auth.CurrentUser(c).ID, in.MemberID, strings.TrimSpace(in.Note), in.Items)
	if err != nil {
		return stockError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (h *Handler) listOpnames(c *fiber.Ctx) error {
	opnames, err := h.svc.ListOpnames(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(opnames)
}

func (h *Handler) getOpname(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	opname, err := h.svc.GetOpname(c.UserContext(), id)
	if err != nil {
		return stockError(err)
	}
	return c.JSON(opname)
}

func (h *Handler) createOpname(c *fiber.Ctx) error {
	var in struct {
		MemberID *int64 `json:"member_id"`
		Note     string `json:"note"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	id, err := h.svc.CreateOpname(c.UserContext(), auth.CurrentUser(c).ID, in.MemberID, strings.TrimSpace(in.Note))
	if err != nil {
		return stockError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (h *Handler) setPhysicalQty(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	productID, err := int64Param(c, "product_id")
	if err != nil {
		return err
	}
	var in struct {
		PhysicalQty *int `json:"physical_qty"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	if in.PhysicalQty != nil && *in.PhysicalQty < 0 {
		return invalid("Jumlah fisik tidak boleh minus")
	}
	if err := h.svc.SetPhysicalQty(c.UserContext(), id, productID, in.PhysicalQty); err != nil {
		return stockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) deleteOpname(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteOpname(c.UserContext(), id); err != nil {
		return stockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) postOpname(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.PostOpname(c.UserContext(), id, auth.CurrentUser(c).ID); err != nil {
		return stockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) stockCard(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	card, err := h.svc.StockCard(c.UserContext(), id)
	if err != nil {
		return stockError(err)
	}
	return c.JSON(card)
}

func stockError(err error) error {
	switch {
	case errors.Is(err, ErrReceiptNotFound), errors.Is(err, ErrOpnameNotFound), errors.Is(err, ErrProductNotFound),
		errors.Is(err, ErrReturnNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrWrongOwner), errors.Is(err, ErrOpnamePosted), errors.Is(err, ErrNoProducts),
		errors.Is(err, ErrNothingCounted), errors.Is(err, ErrMemberNotFound), errors.Is(err, ErrReceiptCancelled):
		return invalid(err.Error())
	}
	return err
}
