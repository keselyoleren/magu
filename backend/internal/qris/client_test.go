package qris

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func sign(orderID, statusCode, amount, key string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + amount + key))
	return hex.EncodeToString(sum[:])
}

func TestChargeAndStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, _, _ := r.BasicAuth(); user != "key" {
			t.Errorf("basic auth user = %q", user)
		}
		switch r.URL.Path {
		case "/v2/charge":
			var body struct {
				PaymentType string `json:"payment_type"`
				Details     struct {
					OrderID string `json:"order_id"`
					Amount  int64  `json:"gross_amount"`
				} `json:"transaction_details"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.PaymentType != "qris" || body.Details.OrderID != "INV1-1" || body.Details.Amount != 15000 {
				t.Errorf("charge body = %+v", body)
			}
			fmt.Fprint(w, `{"status_code":"201","transaction_status":"pending","qr_string":"000201QR"}`)
		case "/v2/INV1-1/status":
			fmt.Fprint(w, `{"status_code":"200","order_id":"INV1-1","gross_amount":"15000.00","transaction_status":"settlement"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"status_code":"404","status_message":"Transaction doesn't exist."}`)
		}
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "key")
	ctx := context.Background()

	qr, err := c.Charge(ctx, "INV1-1", 15000)
	if err != nil || qr != "000201QR" {
		t.Errorf("Charge = %q, %v", qr, err)
	}
	res, err := c.Status(ctx, "INV1-1")
	if err != nil || res.Status != "paid" || res.Amount != 15000 {
		t.Errorf("Status = %+v, %v", res, err)
	}
	if _, err := c.Status(ctx, "NOPE"); err == nil {
		t.Error("Status tagihan tak dikenal seharusnya error")
	}
}

func TestParseNotification(t *testing.T) {
	c := NewClient("http://unused", "key")
	body := func(status, signature string) []byte {
		return []byte(fmt.Sprintf(`{"order_id":"INV1-1","status_code":"200","gross_amount":"15000.00","transaction_status":%q,"signature_key":%q}`, status, signature))
	}
	good := sign("INV1-1", "200", "15000.00", "key")

	res, err := c.ParseNotification(body("settlement", good))
	if err != nil || res.Status != "paid" || res.Amount != 15000 || res.OrderID != "INV1-1" {
		t.Errorf("notifikasi sah = %+v, %v", res, err)
	}
	if res, _ := c.ParseNotification(body("pending", good)); res == nil || res.Status != "" {
		t.Errorf("status pending = %+v", res)
	}
	if _, err := c.ParseNotification(body("settlement", sign("INV1-1", "200", "15000.00", "kunci-lain"))); !errors.Is(err, ErrBadSignature) {
		t.Errorf("signature salah: err = %v", err)
	}
	if NewClient("http://unused", "") != nil {
		t.Error("server key kosong seharusnya menghasilkan client nil")
	}
}
