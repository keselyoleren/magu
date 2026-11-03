package auth

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
)

const cookieName = "session"

type Handler struct {
	svc *Service
}

func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) Login(c *fiber.Ctx) error {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	token, user, err := h.svc.Login(c.UserContext(), in.Username, in.Password)
	if errors.Is(err, ErrInvalidCredentials) {
		return fiber.NewError(fiber.StatusUnauthorized, err.Error())
	}
	if err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HTTPOnly: true,
		Secure:   c.Secure(),
		SameSite: fiber.CookieSameSiteStrictMode,
	})
	return c.JSON(user)
}

func (h *Handler) Logout(c *fiber.Ctx) error {
	if err := h.svc.Logout(c.UserContext(), c.Cookies(cookieName)); err != nil {
		return err
	}
	c.ClearCookie(cookieName)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) Me(c *fiber.Ctx) error {
	return c.JSON(CurrentUser(c))
}

func (h *Handler) UserRoutes(r fiber.Router) {
	r.Get("/users", RequireAdmin, h.listUsers)
	r.Post("/users", RequireAdmin, h.saveUser)
	r.Put("/users/:id", RequireAdmin, h.saveUser)
}

func (h *Handler) listUsers(c *fiber.Ctx) error {
	users, err := h.svc.ListUsers(c.UserContext())
	if err != nil {
		return err
	}
	return c.JSON(users)
}

func (h *Handler) saveUser(c *fiber.Ctx) error {
	var in struct {
		Account
		Password string `json:"password"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	a := in.Account
	a.Username = strings.ToLower(strings.TrimSpace(a.Username))
	a.FullName = strings.TrimSpace(a.FullName)
	creating := c.Method() == fiber.MethodPost
	switch {
	case a.Username == "" || strings.ContainsAny(a.Username, " \t"):
		return invalid("Username wajib diisi, tanpa spasi")
	case a.FullName == "":
		return invalid("Nama wajib diisi")
	case a.Role != "admin" && a.Role != "cashier":
		return invalid("Peran harus admin atau kasir")
	case creating && in.Password == "":
		return invalid("Password wajib diisi")
	case in.Password != "" && len(in.Password) < 8:
		return invalid("Password minimal 8 karakter")
	}

	var err error
	if creating {
		err = h.svc.CreateUser(c.UserContext(), &a, in.Password)
	} else {
		id, perr := c.ParamsInt("id")
		if perr != nil {
			return fiber.ErrNotFound
		}
		a.ID = int64(id)
		me := CurrentUser(c)
		if a.ID == me.ID && (a.Role != me.Role || !a.IsActive) {
			return invalid("Anda tidak bisa menonaktifkan atau mengubah peran akun sendiri")
		}
		if err = h.svc.UpdateUser(c.UserContext(), &a, in.Password); err == nil && a.ID != me.ID {
			err = h.svc.RevokeSessions(c.UserContext(), a.ID)
		}
	}
	switch {
	case errors.Is(err, ErrUserNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrUsernameTaken):
		return invalid(err.Error())
	case err != nil:
		return err
	}
	return c.JSON(a)
}

func invalid(msg string) error {
	return fiber.NewError(fiber.StatusUnprocessableEntity, msg)
}

func (h *Handler) Require(c *fiber.Ctx) error {
	user, err := h.svc.Session(c.UserContext(), c.Cookies(cookieName))
	if errors.Is(err, ErrNoSession) {
		return fiber.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	c.Locals("user", user)
	return c.Next()
}

func RequireAdmin(c *fiber.Ctx) error {
	if CurrentUser(c).Role != "admin" {
		return fiber.ErrForbidden
	}
	return c.Next()
}

func CurrentUser(c *fiber.Ctx) *User {
	return c.Locals("user").(*User)
}
