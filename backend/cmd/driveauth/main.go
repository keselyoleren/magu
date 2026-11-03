// Command driveauth menghubungkan Magu ke akun Google toko, sekali saja, dari komputer yang punya browser.
// Google tidak menerima alamat IP lokal sebagai redirect OAuth, jadi izin diambil di sini lalu hasilnya disalin ke .env server.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"magu/internal/drive"
)

const rootFolderName = "Laporan Magu"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gagal:", err)
		os.Exit(1)
	}
}

func run() error {
	clientID, secret := os.Getenv("GOOGLE_CLIENT_ID"), os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || secret == "" {
		return errors.New("isi GOOGLE_CLIENT_ID dan GOOGLE_CLIENT_SECRET di .env dulu")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	redirect := "http://" + listener.Addr().String()

	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	state := hex.EncodeToString(nonce)

	// prompt=consent: tanpa ini Google tidak memberi refresh token pada izin yang kedua dan seterusnya.
	fmt.Println("Buka tautan ini dan masuk dengan akun Google toko:")
	fmt.Println("https://accounts.google.com/o/oauth2/v2/auth?" + url.Values{
		"client_id":     {clientID},
		"redirect_uri":  {redirect},
		"response_type": {"code"},
		"scope":         {drive.Scope},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}.Encode())

	codes := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if r.URL.Query().Get("state") != state || code == "" {
			http.Error(w, "Izin gagal. Kembali ke terminal dan ulangi.", http.StatusBadRequest)
			return
		}
		fmt.Fprintln(w, "Berhasil. Silakan kembali ke terminal.")
		select {
		case codes <- code:
		default:
		}
	})}
	go server.Serve(listener)
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var code string
	select {
	case code = <-codes:
	case <-ctx.Done():
		return errors.New("tidak ada jawaban dari browser selama 5 menit")
	}

	refresh, err := exchange(ctx, clientID, secret, code, redirect)
	if err != nil {
		return err
	}
	root := os.Getenv("DRIVE_ROOT_FOLDER_ID")
	if root == "" {
		client := drive.NewClient(drive.Config{ClientID: clientID, ClientSecret: secret, RefreshToken: refresh})
		if root, err = client.CreateFolder(ctx, rootFolderName); err != nil {
			return err
		}
	}

	fmt.Printf("\nTambahkan ke .env (lokal dan ~/magu/.env di server):\n\nGOOGLE_REFRESH_TOKEN=%s\nDRIVE_ROOT_FOLDER_ID=%s\n", refresh, root)
	return nil
}

func exchange(ctx context.Context, clientID, secret, code, redirect string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"client_secret": {secret},
		"code":          {code},
		"redirect_uri":  {redirect},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, drive.DefaultTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var out struct {
		RefreshToken string `json:"refresh_token"`
		Description  string `json:"error_description"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.RefreshToken == "" {
		return "", fmt.Errorf("Google tidak memberi refresh token: %s", out.Description)
	}
	return out.RefreshToken, nil
}
