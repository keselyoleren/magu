package sales

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"github.com/gofiber/fiber/v2"

	"magu/internal/auth"
	"magu/internal/qris"
	"magu/internal/realtime"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}$`)

// gateway nil berarti QRIS dimatikan (server key belum diisi).
type Handler struct {
	svc     *Service
	gateway *qris.Client
	hub     *realtime.Hub
}

func NewHandler(svc *Service, gateway *qris.Client, hub *realtime.Hub) *Handler {
	return &Handler{svc: svc, gateway: gateway, hub: hub}
}

func (h *Handler) Routes(r fiber.Router) {
	r.Get("/recap", h.recap)

	r.Post("/checkout", h.checkout)
	r.Get("/transactions", h.list)
	r.Get("/transactions/:id", h.get)
	r.Post("/transactions/:id/payments", h.addPayments)
	r.Post("/transactions/:id/void", h.void)
	r.Post("/payments/:id/check", h.checkPayment)
}

func (h *Handler) validatePayments(payments []PaymentInput) error {
	qrisCount := 0
	for _, p := range payments {
		switch {
		case p.Method == "qris" && h.gateway == nil:
			return invalid("QRIS belum diaktifkan di server")
		case p.Method != "cash" && p.Method != "debit" && p.Method != "qris":
			return invalid("Metode pembayaran tidak dikenal")
		case p.Amount <= 0:
			return invalid("Nominal pembayaran harus lebih dari 0")
		case p.Tendered != nil && *p.Tendered < p.Amount:
			return invalid("Uang diterima kurang dari nominal")
		}
		if p.Method == "qris" {
			qrisCount++
		}
	}
	if qrisCount > 1 {
		return invalid("Hanya satu pembayaran QRIS per transaksi")
	}
	return nil
}

func (h *Handler) checkout(c *fiber.Ctx) error {
	var in CheckoutInput
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	if !uuidPattern.MatchString(in.ClientRef) {
		return invalid("client_ref harus UUID")
	}
	if err := h.validatePayments(in.Payments); err != nil {
		return err
	}

	id, err := h.svc.Checkout(c.UserContext(), auth.CurrentUser(c).ID, in)
	if err != nil {
		return saleError(err)
	}
	return h.respond(c, id, fiber.StatusCreated)
}

func (h *Handler) addPayments(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.ErrNotFound
	}
	var in struct {
		Payments []PaymentInput `json:"payments"`
	}
	if err := c.BodyParser(&in); err != nil {
		return fiber.ErrBadRequest
	}
	if len(in.Payments) == 0 {
		return invalid("Belum ada pembayaran")
	}
	if err := h.validatePayments(in.Payments); err != nil {
		return err
	}

	dropped, err := h.svc.AddPayments(c.UserContext(), int64(id), in.Payments)
	if err != nil {
		return saleError(err)
	}
	h.cancelAtGateway(c.UserContext(), dropped)
	return h.respond(c, int64(id), fiber.StatusOK)
}

// respond membuat tagihan QRIS yang belum ada, lalu mengirim keadaan transaksi terkini.
// Gateway dipanggil setelah transaksi DB selesai supaya koneksi database tidak tertahan menunggu pihak ketiga.
func (h *Handler) respond(c *fiber.Ctx, id int64, status int) error {
	ctx := c.UserContext()
	charges, err := h.svc.UnchargedQRIS(ctx, id)
	if err != nil {
		return err
	}
	for _, charge := range charges {
		qr, err := h.gateway.Charge(ctx, charge.ProviderRef, charge.Amount)
		if err != nil {
			slog.Error("tagihan qris gagal dibuat", "provider_ref", charge.ProviderRef, "error", err)
			err = h.svc.FailPayment(ctx, charge.PaymentID)
		} else {
			err = h.svc.SetQRString(ctx, charge.PaymentID, qr)
		}
		if err != nil {
			return err
		}
	}
	t, err := h.svc.Get(ctx, id)
	if err != nil {
		return err
	}
	return c.Status(status).JSON(t)
}

func (h *Handler) cancelAtGateway(ctx context.Context, providerRefs []string) {
	if h.gateway == nil {
		return
	}
	for _, ref := range providerRefs {
		if err := h.gateway.Cancel(ctx, ref); err != nil {
			slog.Warn("tagihan qris tidak bisa dibatalkan di gateway", "provider_ref", ref, "error", err)
		}
	}
}

// Webhook dipanggil gateway, tanpa sesi; pengamannya adalah signature.
func (h *Handler) Webhook(c *fiber.Ctx) error {
	if h.gateway == nil {
		return fiber.ErrNotFound
	}
	result, err := h.gateway.ParseNotification(c.Body())
	if errors.Is(err, qris.ErrBadSignature) {
		return fiber.ErrUnauthorized
	}
	if err != nil {
		return fiber.ErrBadRequest
	}
	if _, err := h.apply(c.UserContext(), result, c.Body()); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusOK)
}

// checkPayment menanyakan status ke gateway: cadangan bila webhook tidak sampai.
func (h *Handler) checkPayment(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil || h.gateway == nil {
		return fiber.ErrNotFound
	}
	charge, err := h.svc.PendingCharge(c.UserContext(), int64(id))
	if err != nil {
		return saleError(err)
	}
	result, err := h.gateway.Status(c.UserContext(), charge.ProviderRef)
	if err != nil {
		slog.Warn("cek status qris gagal", "provider_ref", charge.ProviderRef, "error", err)
		return fiber.NewError(fiber.StatusBadGateway, "Gateway pembayaran tidak bisa dihubungi")
	}
	update, err := h.apply(c.UserContext(), result, nil)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"changed": update != nil})
}

func (h *Handler) apply(ctx context.Context, result *qris.Result, payload []byte) (*PaymentUpdate, error) {
	if result.Status == "" {
		return nil, nil
	}
	update, err := h.svc.ApplyGatewayResult(ctx, result.OrderID, result.Status, result.Amount, payload)
	if err != nil {
		return nil, err
	}
	if update == nil {
		slog.Warn("kabar qris diabaikan", "order_id", result.OrderID, "status", result.Status, "amount", result.Amount)
		return nil, nil
	}
	h.hub.Broadcast("payment.updated", update)
	return update, nil
}

func (h *Handler) list(c *fiber.Ctx) error {
	date := c.Query("date", time.Now().Format(time.DateOnly))
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return invalid("Tanggal tidak valid")
	}
	transactions, err := h.svc.ListByDate(c.UserContext(), date)
	if err != nil {
		return err
	}
	return c.JSON(transactions)
}

func (h *Handler) get(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.ErrNotFound
	}
	t, err := h.svc.Get(c.UserContext(), int64(id))
	if err != nil {
		return saleError(err)
	}
	return c.JSON(t)
}

// void: admin boleh membatalkan transaksi apa pun; kasir hanya yang belum lunas (pembeli batal bayar).
func (h *Handler) void(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.ErrNotFound
	}
	user := auth.CurrentUser(c)
	if user.Role != "admin" {
		t, err := h.svc.Get(c.UserContext(), int64(id))
		if err != nil {
			return saleError(err)
		}
		if t.Status != "pending" {
			return fiber.NewError(fiber.StatusForbidden, "Transaksi lunas hanya bisa dibatalkan admin")
		}
	}
	dropped, err := h.svc.Void(c.UserContext(), int64(id), user.ID)
	if err != nil {
		return saleError(err)
	}
	h.cancelAtGateway(c.UserContext(), dropped)
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *Handler) recap(c *fiber.Ctx) error {
	date := c.Query("date", time.Now().Format(time.DateOnly))
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return invalid("Tanggal tidak valid")
	}
	recap, err := h.svc.DailyRecap(c.UserContext(), date)
	if err != nil {
		return err
	}
	return c.JSON(recap)
}

func saleError(err error) error {
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrPaymentNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidCart), errors.Is(err, ErrProductNotFound), errors.Is(err, ErrPaymentMismatch),
		errors.Is(err, ErrAlreadyVoid), errors.Is(err, ErrSettled), errors.Is(err, ErrNotPending):
		return invalid(err.Error())
	}
	return err
}

func invalid(msg string) error {
	return fiber.NewError(fiber.StatusUnprocessableEntity, msg)
}
