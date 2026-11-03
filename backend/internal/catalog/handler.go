package catalog

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Routes(r fiber.Router, admin fiber.Handler) {
	r.Get("/catalog", h.snapshot)
	r.Get("/catalog/changes", h.changes)
	r.Get("/products", admin, h.list)
	r.Get("/products/:id", admin, h.get)
	r.Post("/products", admin, h.save)
	// Harus didaftarkan sebelum "/products/:id", kalau tidak "prices" terbaca sebagai id.
	r.Put("/products/prices", admin, h.setPrices)
	r.Put("/products/:id", admin, h.save)
	r.Delete("/products/:id", admin, h.remove)
	r.Post("/products/import", admin, h.importCSV)
	r.Post("/products/barcode", admin, h.nextBarcode)

}

func (h *Handler) snapshot(c *fiber.Ctx) error {
	data, err := h.svc.Snapshot(c.UserContext())
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	c.Set(fiber.HeaderContentEncoding, "gzip")
	return c.Send(data)
}

func (h *Handler) changes(c *fiber.Ctx) error {
	since, err := time.Parse(time.RFC3339Nano, c.Query("since"))
	if err != nil {
		return invalid("Parameter since tidak valid")
	}
	changes, err := h.svc.Changes(c.UserContext(), since, int64(c.QueryInt("after_id")))
	if err != nil {
		return err
	}
	// Produk yang dihapus tidak muncul di delta; klien membandingkan jumlah ini untuk tahu katalognya perlu diambil ulang.
	active, err := h.svc.ActiveCount(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": changes, "has_more": len(changes) == deltaLimit, "now": time.Now(), "active_count": active})
}

func (h *Handler) list(c *fiber.Ctx) error {
	products, hasMore, err := h.svc.List(c.UserContext(),
		strings.TrimSpace(c.Query("q")), int64(c.QueryInt("member_id")), max(c.QueryInt("page"), 0), c.QueryInt("per_page"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"items": products, "has_more": hasMore})
}

func (h *Handler) get(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.ErrNotFound
	}
	p, err := h.svc.Get(c.UserContext(), int64(id))
	if errors.Is(err, ErrProductNotFound) {
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}
	if err != nil {
		return err
	}
	return c.JSON(p)
}

func (h *Handler) remove(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || id <= 0 {
		return fiber.ErrNotFound
	}
	err = h.svc.Delete(c.UserContext(), int64(id))
	switch {
	case errors.Is(err, ErrProductNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrProductInUse):
		return invalid(err.Error())
	case err != nil:
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) setPrices(c *fiber.Ctx) error {
	var changes []PriceChange
	if err := c.BodyParser(&changes); err != nil {
		return fiber.ErrBadRequest
	}
	for _, ch := range changes {
		if ch.Price < 0 {
			return invalid("Harga tidak boleh minus")
		}
	}
	updated, err := h.svc.SetPrices(c.UserContext(), changes)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"updated": updated})
}

func (h *Handler) nextBarcode(c *fiber.Ctx) error {
	var in struct {
		MemberID *int64 `json:"member_id"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	barcode, err := h.svc.NextBarcode(c.UserContext(), in.MemberID)
	if errors.Is(err, ErrMemberNotFound) {
		return invalid(err.Error())
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"barcode": barcode})
}

func (h *Handler) save(c *fiber.Ctx) error {
	var p Product
	if err := c.BodyParser(&p); err != nil {
		return fiber.ErrBadRequest
	}
	p.Name = strings.TrimSpace(p.Name)
	p.Barcode = strings.TrimSpace(p.Barcode)
	switch {
	case p.Name == "":
		return invalid("Nama wajib diisi")
	case p.Price < 0:
		return invalid("Harga tidak boleh minus")
	case p.CommissionBps != nil && p.MemberID == nil:
		return invalid("Barang milik toko tidak memakai komisi")
	case p.CommissionBps != nil && (*p.CommissionBps < 0 || *p.CommissionBps > 10000):
		return invalid("Komisi harus antara 0 dan 100%")
	}

	var err error
	if c.Method() == fiber.MethodPost {
		err = h.svc.Create(c.UserContext(), &p)
	} else {
		id, perr := c.ParamsInt("id")
		if perr != nil || id <= 0 {
			return fiber.ErrNotFound
		}
		p.ID = int64(id)
		err = h.svc.Update(c.UserContext(), &p)
	}
	switch {
	case errors.Is(err, ErrProductNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrBarcodeTaken), errors.Is(err, ErrMemberNotFound):
		return invalid(err.Error())
	case err != nil:
		return err
	}
	return c.JSON(p)
}

func (h *Handler) importCSV(c *fiber.Ctx) error {
	count, problems, err := h.svc.Import(c.UserContext(), c.Body())
	if err != nil {
		return err
	}
	if len(problems) > 0 {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"error":   "Berkas tidak bisa diimpor",
			"details": problems,
		})
	}
	return c.JSON(fiber.Map{"imported": count})
}

func invalid(msg string) error {
	return fiber.NewError(fiber.StatusUnprocessableEntity, msg)
}
