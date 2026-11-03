package consignment

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"magu/internal/auth"
)

func (h *Handler) settlementRoutes(r fiber.Router, admin fiber.Handler) {
	r.Get("/settlements/outstanding", admin, h.outstanding)
	r.Get("/settlements", admin, h.listSettlements)
	r.Post("/settlements", admin, h.createSettlement)
	r.Get("/settlements/:id", admin, h.getSettlement)
	r.Post("/settlements/:id/pay", admin, h.paySettlement)
	r.Delete("/settlements/:id", admin, h.deleteSettlement)
	r.Get("/reports/sales", admin, h.salesReport)
	r.Get("/dashboard", admin, h.dashboard)
}

func (h *Handler) dashboard(c *fiber.Ctx) error {
	d, err := h.svc.Dashboard(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(d)
}

func (h *Handler) outstanding(c *fiber.Ctx) error {
	rows, err := h.svc.Outstanding(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(rows)
}

func (h *Handler) listSettlements(c *fiber.Ctx) error {
	rows, err := h.svc.ListSettlements(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(rows)
}

func (h *Handler) getSettlement(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	st, err := h.svc.GetSettlement(c.UserContext(), id)
	if err != nil {
		return settlementError(err)
	}
	return c.JSON(st)
}

func (h *Handler) createSettlement(c *fiber.Ctx) error {
	var in struct {
		MemberID int64  `json:"member_id"`
		Until    string `json:"until"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	until, err := time.Parse(time.DateOnly, in.Until)
	if err != nil {
		return invalid("Tanggal batas tidak valid")
	}
	if in.MemberID <= 0 {
		return invalid("Pilih member")
	}
	id, err := h.svc.CreateSettlement(c.UserContext(), auth.CurrentUser(c).ID, in.MemberID, until)
	if err != nil {
		return settlementError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"id": id})
}

func (h *Handler) paySettlement(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.PaySettlement(c.UserContext(), id); err != nil {
		return settlementError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) deleteSettlement(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	if err := h.svc.DeleteSettlement(c.UserContext(), id); err != nil {
		return settlementError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) salesReport(c *fiber.Ctx) error {
	from, errFrom := time.Parse(time.DateOnly, c.Query("from"))
	to, errTo := time.Parse(time.DateOnly, c.Query("to"))
	if errFrom != nil || errTo != nil || to.Before(from) {
		return invalid("Rentang tanggal tidak valid")
	}
	daily, owners, err := h.svc.SalesReport(c.UserContext(), from, to)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"daily": daily, "owners": owners})
}

func settlementError(err error) error {
	switch {
	case errors.Is(err, ErrSettlementNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrSettlementPaid), errors.Is(err, ErrNothingToSettle), errors.Is(err, ErrMemberNotFound):
		return invalid(err.Error())
	}
	return err
}
