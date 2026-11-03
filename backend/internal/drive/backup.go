package drive

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"time"
)

const (
	backupFolder = "Backup Database"
	keepBackups  = 30
	// Berkas yang baru berubah dilewati karena pg_dump mungkin masih menulisnya.
	dumpSettle = 2 * time.Minute
)

// Hanya dump harian; dump manual seperti magu-sebelum-drive.dump tetap di server.
var dumpName = regexp.MustCompile(`^magu-\d{4}-\d{2}-\d{2}\.dump$`)

// PushBackups menyamakan folder backup di Drive dengan dump harian terbaru di dir, jadi hari yang
// terlewat saat internet mati ikut terkirim. Mengembalikan nama berkas yang diunggah.
func (c *Client) PushBackups(ctx context.Context, dir string, now time.Time) ([]string, error) {
	if c.cfg.RootFolder == "" {
		return nil, errors.New("DRIVE_ROOT_FOLDER_ID kosong")
	}
	dumps, err := localDumps(dir, now)
	if err != nil {
		return nil, err
	}
	folderID, err := c.findFile(ctx, c.cfg.RootFolder, backupFolder)
	if err != nil {
		return nil, err
	}
	if folderID == "" {
		if folderID, err = c.CreateFolder(ctx, backupFolder); err != nil {
			return nil, err
		}
	}
	remote, err := c.listFiles(ctx, folderID)
	if err != nil {
		return nil, err
	}
	sizes := make(map[string]int64, len(remote))
	for _, f := range remote {
		size, _ := strconv.ParseInt(f.Size, 10, 64)
		sizes[f.Name] = size
	}

	var uploaded []string
	for _, d := range dumps {
		if size, ok := sizes[d.name]; ok && size == d.size {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, d.name))
		if err != nil {
			return uploaded, err
		}
		if err := c.Upload(ctx, folderID, d.name, "application/octet-stream", data); err != nil {
			return uploaded, fmt.Errorf("%s: %w", d.name, err)
		}
		uploaded = append(uploaded, d.name)
		if _, ok := sizes[d.name]; !ok {
			remote = append(remote, driveFile{Name: d.name})
		}
	}

	slices.SortFunc(remote, func(a, b driveFile) int { return cmp.Compare(b.Name, a.Name) })
	for _, f := range remote[min(keepBackups, len(remote)):] {
		if f.ID == "" {
			continue
		}
		if err := c.trash(ctx, f.ID); err != nil {
			return uploaded, err
		}
	}
	return uploaded, nil
}

type dump struct {
	name string
	size int64
}

func localDumps(dir string, now time.Time) ([]dump, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dumps []dump
	for _, e := range entries {
		if !dumpName.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return nil, err
		}
		if info.Size() == 0 || now.Sub(info.ModTime()) < dumpSettle {
			continue
		}
		dumps = append(dumps, dump{e.Name(), info.Size()})
	}
	slices.SortFunc(dumps, func(a, b dump) int { return cmp.Compare(b.name, a.name) })
	return dumps[:min(keepBackups, len(dumps))], nil
}
