package consignment

import (
	"errors"
	"net/mail"
	"strings"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	svc       *Service
	storeName string
}

// storeName dicantumkan di kepala laporan Excel.
func NewHandler(svc *Service, storeName string) *Handler {
	return &Handler{svc: svc, storeName: storeName}
}

func (h *Handler) Routes(r fiber.Router, admin fiber.Handler) {
	r.Get("/members", admin, h.listMembers)
	r.Post("/members", admin, h.saveMember)
	r.Put("/members/:id", admin, h.saveMember)
	h.stockRoutes(r, admin)
	h.settlementRoutes(r, admin)
	h.excelRoutes(r, admin)
	h.driveRoutes(r, admin)
}

func (h *Handler) listMembers(c *fiber.Ctx) error {
	members, err := h.svc.ListMembers(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(members)
}

func (h *Handler) saveMember(c *fiber.Ctx) error {
	var m Member
	if err := c.BodyParser(&m); err != nil {
		return fiber.ErrBadRequest
	}
	m.Code = strings.TrimSpace(m.Code)
	m.Name = strings.TrimSpace(m.Name)
	m.Email = strings.ToLower(strings.TrimSpace(m.Email))
	if m.Email != "" {
		if _, err := mail.ParseAddress(m.Email); err != nil {
			return invalid("Email tidak valid")
		}
	}
	switch {
	case m.Code == "":
		return invalid("Kode wajib diisi")
	case m.Name == "":
		return invalid("Nama wajib diisi")
	case m.CommissionBps < 0 || m.CommissionBps > 10000:
		return invalid("Komisi harus antara 0 dan 100%")
	}

	var err error
	if c.Method() == fiber.MethodPost {
		err = h.svc.CreateMember(c.UserContext(), &m)
	} else {
		if m.ID, err = int64Param(c, "id"); err != nil {
			return err
		}
		err = h.svc.UpdateMember(c.UserContext(), &m)
	}
	switch {
	case errors.Is(err, ErrMemberNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrMemberCodeTaken):
		return invalid(err.Error())
	case err != nil:
		return err
	}
	return c.JSON(m)
}

func invalid(msg string) error {
	return fiber.NewError(fiber.StatusUnprocessableEntity, msg)
}

func int64Param(c *fiber.Ctx, name string) (int64, error) {
	id, err := c.ParamsInt(name)
	if err != nil || id <= 0 {
		return 0, fiber.ErrNotFound
	}
	return int64(id), nil
}
