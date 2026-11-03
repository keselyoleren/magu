package printer

import (
	"bytes"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func send(t *testing.T, target string) int {
	app := fiber.New()
	app.Post("/print", Print(target))
	res, err := app.Test(httptest.NewRequest("POST", "/print", bytes.NewReader([]byte{0x1b, 0x40, 'A'})))
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode
}

func TestDevice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lp0")
	os.WriteFile(path, nil, 0o600)
	if code := send(t, path); code != 204 {
		t.Fatal(code)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, []byte{0x1b, 0x40, 'A'}) {
		t.Fatal(got)
	}
	if code := send(t, filepath.Join(t.TempDir(), "missing")); code != 502 {
		t.Fatal(code)
	}
}

func TestTCP(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	got := make(chan []byte, 1)
	go func() {
		c, _ := ln.Accept()
		b, _ := io.ReadAll(c)
		got <- b
	}()
	if code := send(t, "tcp://"+ln.Addr().String()); code != 204 {
		t.Fatal(code)
	}
	if b := <-got; !bytes.Equal(b, []byte{0x1b, 0x40, 'A'}) {
		t.Fatal(b)
	}
}
