package drive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

type fakeBackupDrive struct {
	folderMade int
	files      map[string]string
	ids        map[string]string
	trashed    []string
}

func (f *fakeBackupDrive) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/token":
		fmt.Fprint(w, `{"access_token":"abc","expires_in":3600}`)
	case r.Method == http.MethodGet:
		q := r.URL.Query().Get("q")
		if strings.Contains(q, "name = ") {
			if strings.Contains(q, backupFolder) && f.folderMade > 0 {
				fmt.Fprint(w, `{"files":[{"id":"backups"}]}`)
				return
			}
			fmt.Fprint(w, `{"files":[]}`)
			return
		}
		type item struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Size string `json:"size"`
		}
		list := []item{}
		for name, data := range f.files {
			list = append(list, item{f.ids[name], name, fmt.Sprint(len(data))})
		}
		json.NewEncoder(w).Encode(map[string]any{"files": list})
	case r.Method == http.MethodPost && r.URL.Path == "/drive/v3/files":
		f.folderMade++
		fmt.Fprint(w, `{"id":"backups"}`)
	case r.Method == http.MethodPost && r.URL.Path == "/upload/drive/v3/files":
		_, params, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		reader := multipart.NewReader(r.Body, params["boundary"])
		part, _ := reader.NextPart()
		var meta struct {
			Name string `json:"name"`
		}
		json.NewDecoder(part).Decode(&meta)
		part, _ = reader.NextPart()
		data, _ := io.ReadAll(part)
		f.files[meta.Name] = string(data)
		f.ids[meta.Name] = "id-" + meta.Name
		fmt.Fprint(w, `{"id":"x"}`)
	case r.Method == http.MethodPatch:
		name := strings.TrimPrefix(r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:], "id-")
		f.trashed = append(f.trashed, name)
		delete(f.files, name)
		fmt.Fprint(w, `{}`)
	default:
		t := fmt.Sprintf("permintaan tak terduga: %s %s", r.Method, r.URL)
		http.Error(w, t, http.StatusTeapot)
	}
}

func writeDump(t *testing.T, dir, name, data string, age time.Duration) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	stamp := time.Now().Add(-age)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
}

func TestPushBackups(t *testing.T) {
	fake := &fakeBackupDrive{files: map[string]string{}, ids: map[string]string{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	c := NewClient(Config{ClientID: "id", ClientSecret: "secret", RefreshToken: "refresh", RootFolder: "root1", APIBase: srv.URL, TokenURL: srv.URL + "/token"})
	ctx := context.Background()
	dir := t.TempDir()

	writeDump(t, dir, "magu-2026-11-01.dump", "satu", time.Hour)
	writeDump(t, dir, "magu-2026-11-02.dump", "dua", time.Hour)
	writeDump(t, dir, "magu-2026-11-03.dump", "ditulis", time.Second)
	writeDump(t, dir, "magu-2026-11-04.dump", "", time.Hour)
	writeDump(t, dir, "magu-sebelum-drive.dump", "manual", time.Hour)

	got, err := c.PushBackups(ctx, dir, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, []string{"magu-2026-11-02.dump", "magu-2026-11-01.dump"}) || fake.folderMade != 1 {
		t.Errorf("unggahan pertama = %v, %d folder", got, fake.folderMade)
	}

	if got, err = c.PushBackups(ctx, dir, time.Now()); err != nil || len(got) != 0 || fake.folderMade != 1 {
		t.Errorf("tanpa perubahan = %v, %v, %d folder", got, err, fake.folderMade)
	}

	for day := 1; day <= keepBackups; day++ {
		writeDump(t, dir, fmt.Sprintf("magu-2026-12-%02d.dump", day), "isi", time.Hour)
	}
	if _, err := c.PushBackups(ctx, dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(fake.files) != keepBackups {
		t.Errorf("tersimpan %d berkas", len(fake.files))
	}
	slices.Sort(fake.trashed)
	if !slices.Equal(fake.trashed, []string{"magu-2026-11-01.dump", "magu-2026-11-02.dump"}) {
		t.Errorf("dibuang = %v", fake.trashed)
	}
	if _, err := c.PushBackups(ctx, dir, time.Now()); err != nil || len(fake.trashed) != 2 {
		t.Errorf("putaran berikutnya: %v, dibuang %v", err, fake.trashed)
	}
}
