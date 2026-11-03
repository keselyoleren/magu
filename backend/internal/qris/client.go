// Package qris berbicara dengan payment gateway (Midtrans Core API) untuk pembayaran QRIS.
package qris

import (
	"bytes"
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var ErrBadSignature = errors.New("signature notifikasi tidak cocok")

type Client struct {
	baseURL   string
	serverKey string
	http      *http.Client
}

// NewClient mengembalikan nil bila serverKey kosong; pemanggil memperlakukan nil sebagai "QRIS dimatikan".
func NewClient(baseURL, serverKey string) *Client {
	if serverKey == "" {
		return nil
	}
	return &Client{
		baseURL:   strings.TrimRight(baseURL, "/"),
		serverKey: serverKey,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Result adalah keadaan satu tagihan. Status: "paid", "expired", "failed", atau "" bila belum final.
type Result struct {
	OrderID string
	Status  string
	Amount  int64
}

type response struct {
	StatusCode        string `json:"status_code"`
	StatusMessage     string `json:"status_message"`
	OrderID           string `json:"order_id"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	QRString          string `json:"qr_string"`
}

var statusMap = map[string]string{
	"settlement": "paid",
	"expire":     "expired",
	"cancel":     "failed",
	"deny":       "failed",
}

func (r response) result() (*Result, error) {
	res := &Result{OrderID: r.OrderID, Status: statusMap[r.TransactionStatus]}
	rupiah, _, _ := strings.Cut(r.GrossAmount, ".")
	var err error
	if res.Amount, err = strconv.ParseInt(rupiah, 10, 64); err != nil {
		return nil, fmt.Errorf("gross_amount tidak valid: %q", r.GrossAmount)
	}
	return res, nil
}

func (c *Client) call(ctx context.Context, method, path string, body any) (*response, error) {
	var payload bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&payload).Encode(body); err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, &payload)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.serverKey, "")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var out response
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("gateway membalas HTTP %d yang tidak bisa dibaca", res.StatusCode)
	}
	if !strings.HasPrefix(out.StatusCode, "2") {
		return nil, fmt.Errorf("gateway menolak (%s): %s", out.StatusCode, out.StatusMessage)
	}
	return &out, nil
}

// Charge membuat tagihan QRIS dan mengembalikan isi QR yang ditampilkan ke pembeli.
func (c *Client) Charge(ctx context.Context, orderID string, amount int64) (string, error) {
	out, err := c.call(ctx, http.MethodPost, "/v2/charge", map[string]any{
		"payment_type":        "qris",
		"transaction_details": map[string]any{"order_id": orderID, "gross_amount": amount},
	})
	if err != nil {
		return "", err
	}
	if out.QRString == "" {
		return "", errors.New("gateway tidak mengembalikan qr_string")
	}
	return out.QRString, nil
}

func (c *Client) Status(ctx context.Context, orderID string) (*Result, error) {
	out, err := c.call(ctx, http.MethodGet, "/v2/"+orderID+"/status", nil)
	if err != nil {
		return nil, err
	}
	return out.result()
}

func (c *Client) Cancel(ctx context.Context, orderID string) error {
	_, err := c.call(ctx, http.MethodPost, "/v2/"+orderID+"/cancel", nil)
	return err
}

// ParseNotification memverifikasi signature webhook: SHA512(order_id + status_code + gross_amount + server_key).
func (c *Client) ParseNotification(body []byte) (*Result, error) {
	var n response
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, err
	}
	sum := sha512.Sum512([]byte(n.OrderID + n.StatusCode + n.GrossAmount + c.serverKey))
	want := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(want), []byte(n.SignatureKey)) != 1 {
		return nil, ErrBadSignature
	}
	return n.result()
}
