package consignment

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"

	"magu/internal/drive"
)

func (h *Handler) driveRoutes(r fiber.Router, admin fiber.Handler) {
	r.Post("/members/:id/drive", admin, h.syncMemberFolder)
	r.Post("/settlements/:id/drive", admin, h.sendSettlement)
	r.Post("/reports/stock/drive", admin, h.sendStockReport)
}

func (h *Handler) syncMemberFolder(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	folderID, err := h.svc.SyncMemberFolder(c.UserContext(), id)
	if err != nil {
		return driveError(err)
	}
	return c.JSON(fiber.Map{"folder_id": folderID})
}

// Hanya yang sudah dibayar: draft masih bisa dihapus, dan berkasnya akan tertinggal di folder member.
func (h *Handler) sendSettlement(c *fiber.Ctx) error {
	id, err := int64Param(c, "id")
	if err != nil {
		return err
	}
	ctx := c.UserContext()
	st, err := h.svc.GetSettlement(ctx, id)
	if err != nil {
		return settlementError(err)
	}
	if st.Status != "paid" {
		return invalid("Tandai dibayar dulu sebelum dikirim ke Drive")
	}
	book, filename := h.settlementBook(st)
	data, err := book.Bytes()
	if err != nil {
		return err
	}
	folderID, err := h.svc.SendToMemberFolder(ctx, st.MemberID, filename, data)
	if err != nil {
		return driveError(err)
	}
	if err := h.svc.MarkSettlementSent(ctx, id); err != nil {
		return err
	}
	return c.JSON(fiber.Map{"folder_id": folderID})
}

func (h *Handler) sendStockReport(c *fiber.Ctx) error {
	from, errFrom := time.Parse(time.DateOnly, c.Query("from"))
	to, errTo := time.Parse(time.DateOnly, c.Query("to"))
	if errFrom != nil || errTo != nil {
		return invalid("Rentang tanggal tidak valid")
	}
	id := int64(c.QueryInt("member_id"))
	if id <= 0 {
		return invalid("Pilih member")
	}
	ctx := c.UserContext()
	book, filename, err := h.stockBook(ctx, &id, from, to)
	if err != nil {
		return err
	}
	data, err := book.Bytes()
	if err != nil {
		return err
	}
	folderID, err := h.svc.SendToMemberFolder(ctx, id, filename, data)
	if err != nil {
		return driveError(err)
	}
	return c.JSON(fiber.Map{"folder_id": folderID})
}

func driveError(err error) error {
	var failed *drive.Error
	switch {
	case errors.Is(err, ErrMemberNotFound):
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	case errors.Is(err, ErrDriveDisabled):
		return invalid(err.Error())
	case errors.Is(err, drive.ErrAuthExpired):
		slog.Error("izin google drive habis", "error", err)
		return fiber.NewError(fiber.StatusBadGateway, "Izin Google Drive sudah habis. Perbarui dengan make drive-auth, lalu kirim ulang.")
	case errors.As(err, &failed):
		slog.Error("google drive gagal", "status", failed.Status, "message", failed.Message)
		return fiber.NewError(fiber.StatusBadGateway, "Google Drive tidak bisa dihubungi. Cek internet lalu coba lagi.")
	}
	return err
}
