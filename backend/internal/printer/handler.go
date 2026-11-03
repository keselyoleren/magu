package printer

import (
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Print meneruskan byte ESC/POS mentah ke printer. Tujuan datang dari konfigurasi server,
// bukan dari request, supaya endpoint ini tidak bisa dipakai menulis ke file atau host lain.
//
// target berupa "tcp://192.168.1.50:9100" untuk printer LAN, atau path perangkat untuk printer USB
// yang menempel di PC server, mis. "/dev/pos-printer".
func Print(target string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if target == "" {
			return fiber.NewError(fiber.StatusServiceUnavailable, "printer belum diatur di server (PRINTER_TARGET)")
		}
		w, err := open(target)
		if err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "printer tidak terjangkau")
		}
		defer w.Close()

		if _, err := w.Write(c.Body()); err != nil {
			return fiber.NewError(fiber.StatusBadGateway, "gagal mengirim ke printer")
		}
		return c.SendStatus(fiber.StatusNoContent)
	}
}

func open(target string) (io.WriteCloser, error) {
	addr, isTCP := strings.CutPrefix(target, "tcp://")
	if !isTCP {
		return os.OpenFile(target, os.O_WRONLY, 0)
	}
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return nil, err
	}
	conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return conn, nil
}
