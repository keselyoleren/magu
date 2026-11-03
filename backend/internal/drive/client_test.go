package drive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadCreatesThenReplaces(t *testing.T) {
	var tokens int
	var queries []string
	files := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokens++
			r.ParseForm()
			if r.Form.Get("refresh_token") != "refresh" || r.Form.Get("grant_type") != "refresh_token" {
				t.Errorf("form token = %v", r.Form)
			}
			fmt.Fprint(w, `{"access_token":"abc","expires_in":3600}`)
			return
		}
		if r.Header.Get("Authorization") != "Bearer abc" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/drive/v3/files":
			queries = append(queries, r.URL.Query().Get("q"))
			if len(files) == 0 {
				fmt.Fprint(w, `{"files":[]}`)
				return
			}
			fmt.Fprint(w, `{"files":[{"id":"file1"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
			_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			reader := multipart.NewReader(r.Body, params["boundary"])
			metaPart, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			var meta struct {
				Name    string   `json:"name"`
				Parents []string `json:"parents"`
			}
			json.NewDecoder(metaPart).Decode(&meta)
			if meta.Name != "o'brien.xlsx" || len(meta.Parents) != 1 || meta.Parents[0] != "folder1" {
				t.Errorf("metadata = %+v", meta)
			}
			dataPart, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			data, _ := io.ReadAll(dataPart)
			files["file1"] = string(data)
			fmt.Fprint(w, `{"id":"file1"}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/upload/drive/v3/files/file1":
			data, _ := io.ReadAll(r.Body)
			files["file1"] = string(data)
			fmt.Fprint(w, `{"id":"file1"}`)
		default:
			t.Errorf("permintaan tak terduga: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusTeapot)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", APIBase: srv.URL, TokenURL: srv.URL + "/token"})
	ctx := context.Background()
	if err := c.Upload(ctx, "folder1", "o'brien.xlsx", "application/xlsx", []byte("satu")); err != nil {
		t.Fatal(err)
	}
	if err := c.Upload(ctx, "folder1", "o'brien.xlsx", "application/xlsx", []byte("dua")); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files["file1"] != "dua" {
		t.Errorf("berkas = %v", files)
	}
	if tokens != 1 {
		t.Errorf("token diminta %d kali", tokens)
	}
	if !strings.Contains(queries[0], `name = 'o\'brien.xlsx'`) {
		t.Errorf("q = %q", queries[0])
	}
}

func TestShareAndUnshare(t *testing.T) {
	gone := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			fmt.Fprint(w, `{"access_token":"abc","expires_in":3600}`)
		case r.Method == http.MethodPost && r.URL.Path == "/drive/v3/files":
			var meta map[string]any
			json.NewDecoder(r.Body).Decode(&meta)
			if meta["mimeType"] != folderType || meta["parents"].([]any)[0] != "root1" {
				t.Errorf("folder = %v", meta)
			}
			fmt.Fprint(w, `{"id":"folder1"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/drive/v3/files/folder1/permissions":
			var perm map[string]string
			json.NewDecoder(r.Body).Decode(&perm)
			if perm["role"] != "reader" || perm["emailAddress"] != "sari@example.com" {
				t.Errorf("izin = %v", perm)
			}
			fmt.Fprint(w, `{"id":"perm1"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/drive/v3/files/folder1/permissions/perm1":
			if gone {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			gone = true
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"error":"dilarang"}`)
		}
	}))
	defer srv.Close()

	c := NewClient(Config{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", RootFolder: "root1", APIBase: srv.URL, TokenURL: srv.URL + "/token"})
	ctx := context.Background()
	folder, err := c.CreateFolder(ctx, "M1 - Bu Sari")
	if err != nil || folder != "folder1" {
		t.Fatalf("CreateFolder = %q, %v", folder, err)
	}
	perm, err := c.Share(ctx, folder, "sari@example.com")
	if err != nil || perm != "perm1" {
		t.Fatalf("Share = %q, %v", perm, err)
	}
	if err := c.Unshare(ctx, folder, perm); err != nil {
		t.Error(err)
	}
	if err := c.Unshare(ctx, folder, perm); err != nil {
		t.Errorf("izin yang sudah dicabut: %v", err)
	}
	if err := c.Unshare(ctx, "lain", perm); err == nil {
		t.Error("403 seharusnya error")
	}
	if NewClient(Config{ClientID: "id"}) != nil {
		t.Error("kredensial tidak lengkap seharusnya menghasilkan client nil")
	}
}

func TestExpiredRefreshToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":"invalid_grant","error_description":"Token has been expired or revoked."}`)
	}))
	defer srv.Close()

	c := NewClient(Config{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", APIBase: srv.URL, TokenURL: srv.URL + "/token"})
	if _, err := c.CreateFolder(context.Background(), "x"); !errors.Is(err, ErrAuthExpired) {
		t.Errorf("err = %v", err)
	}
}
