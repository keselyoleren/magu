// Package drive menyimpan laporan ke Google Drive akun toko.
package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	Scope           = "https://www.googleapis.com/auth/drive.file"
	DefaultTokenURL = "https://oauth2.googleapis.com/token"
	defaultAPIBase  = "https://www.googleapis.com"
	folderType      = "application/vnd.google-apps.folder"
)

type Config struct {
	ClientID     string
	ClientSecret string
	RefreshToken string
	// RootFolder kosong = folder dibuat langsung di My Drive.
	RootFolder string
	APIBase    string
	TokenURL   string
	// Timeout kosong = 30 detik, cukup untuk laporan.
	Timeout time.Duration
}

type Client struct {
	cfg  Config
	http *http.Client

	mu      sync.Mutex
	access  string
	expires time.Time
}

// ErrAuthExpired berarti refresh token ditolak Google, mis. dicabut atau password akun diganti.
var ErrAuthExpired = errors.New("izin google drive habis")

// Error adalah kegagalan bicara dengan Google. Status 0 berarti server tidak terjangkau.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("google drive %d: %s", e.Status, e.Message)
}

// NewClient mengembalikan nil bila kredensial belum lengkap; pemanggil memperlakukan nil sebagai "Drive dimatikan".
func NewClient(cfg Config) *Client {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RefreshToken == "" {
		return nil
	}
	if cfg.APIBase == "" {
		cfg.APIBase = defaultAPIBase
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = DefaultTokenURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	cfg.APIBase = strings.TrimRight(cfg.APIBase, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

func FolderURL(id string) string {
	return "https://drive.google.com/drive/folders/" + id
}

func (c *Client) token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.access != "" && time.Now().Before(c.expires) {
		return c.access, nil
	}
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {c.cfg.ClientID},
		"client_secret": {c.cfg.ClientSecret},
		"refresh_token": {c.cfg.RefreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := c.send(req, &out); err != nil {
		if e, ok := err.(*Error); ok && strings.Contains(e.Message, "invalid_grant") {
			return "", fmt.Errorf("%w: %s", ErrAuthExpired, e.Message)
		}
		return "", err
	}
	c.access = out.AccessToken
	c.expires = time.Now().Add(time.Duration(out.ExpiresIn-60) * time.Second)
	return c.access, nil
}

func (c *Client) send(req *http.Request, out any) error {
	res, err := c.http.Do(req)
	if err != nil {
		return &Error{Message: err.Error()}
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return &Error{Status: res.StatusCode, Message: string(body)}
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return &Error{Status: res.StatusCode, Message: "jawaban tidak terbaca: " + err.Error()}
	}
	return nil
}

func (c *Client) call(ctx context.Context, method, path, contentType string, body []byte, out any) error {
	token, err := c.token(ctx)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.send(req, out)
}

func (c *Client) CreateFolder(ctx context.Context, name string) (string, error) {
	meta := map[string]any{"name": name, "mimeType": folderType}
	if c.cfg.RootFolder != "" {
		meta["parents"] = []string{c.cfg.RootFolder}
	}
	body, err := json.Marshal(meta)
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	err = c.call(ctx, http.MethodPost, "/drive/v3/files?fields=id", "application/json", body, &out)
	return out.ID, err
}

// Share memberi email itu akses baca; Google mengirim undangan berisi tautan foldernya.
func (c *Client) Share(ctx context.Context, folderID, email string) (string, error) {
	body, err := json.Marshal(map[string]string{"type": "user", "role": "reader", "emailAddress": email})
	if err != nil {
		return "", err
	}
	var out struct {
		ID string `json:"id"`
	}
	path := "/drive/v3/files/" + url.PathEscape(folderID) + "/permissions?fields=id"
	err = c.call(ctx, http.MethodPost, path, "application/json", body, &out)
	return out.ID, err
}

func (c *Client) Unshare(ctx context.Context, folderID, permissionID string) error {
	path := "/drive/v3/files/" + url.PathEscape(folderID) + "/permissions/" + url.PathEscape(permissionID)
	err := c.call(ctx, http.MethodDelete, path, "", nil, nil)
	if e, ok := err.(*Error); ok && e.Status == http.StatusNotFound {
		return nil
	}
	return err
}

// Upload menimpa berkas bernama sama di folder itu, supaya kirim ulang tidak menghasilkan salinan.
func (c *Client) Upload(ctx context.Context, folderID, name, contentType string, data []byte) error {
	id, err := c.findFile(ctx, folderID, name)
	if err != nil {
		return err
	}
	if id != "" {
		return c.call(ctx, http.MethodPatch, "/upload/drive/v3/files/"+url.PathEscape(id)+"?uploadType=media", contentType, data, nil)
	}

	meta, err := json.Marshal(map[string]any{"name": name, "parents": []string{folderID}})
	if err != nil {
		return err
	}
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, part := range []struct {
		contentType string
		data        []byte
	}{{"application/json; charset=UTF-8", meta}, {contentType, data}} {
		p, err := w.CreatePart(textproto.MIMEHeader{"Content-Type": {part.contentType}})
		if err != nil {
			return err
		}
		if _, err := p.Write(part.data); err != nil {
			return err
		}
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.call(ctx, http.MethodPost, "/upload/drive/v3/files?uploadType=multipart", "multipart/related; boundary="+w.Boundary(), body.Bytes(), nil)
}

func (c *Client) findFile(ctx context.Context, folderID, name string) (string, error) {
	q := fmt.Sprintf("name = %s and %s in parents and trashed = false", quote(name), quote(folderID))
	path := "/drive/v3/files?" + url.Values{"q": {q}, "fields": {"files(id)"}, "pageSize": {"1"}}.Encode()
	var out struct {
		Files []struct {
			ID string `json:"id"`
		} `json:"files"`
	}
	if err := c.call(ctx, http.MethodGet, path, "", nil, &out); err != nil {
		return "", err
	}
	if len(out.Files) == 0 {
		return "", nil
	}
	return out.Files[0].ID, nil
}

type driveFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Size string `json:"size"`
}

func (c *Client) listFiles(ctx context.Context, folderID string) ([]driveFile, error) {
	q := fmt.Sprintf("%s in parents and trashed = false", quote(folderID))
	path := "/drive/v3/files?" + url.Values{"q": {q}, "fields": {"files(id,name,size)"}, "pageSize": {"1000"}}.Encode()
	var out struct {
		Files []driveFile `json:"files"`
	}
	err := c.call(ctx, http.MethodGet, path, "", nil, &out)
	return out.Files, err
}

func (c *Client) trash(ctx context.Context, id string) error {
	return c.call(ctx, http.MethodPatch, "/drive/v3/files/"+url.PathEscape(id), "application/json", []byte(`{"trashed":true}`), nil)
}

func quote(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}
