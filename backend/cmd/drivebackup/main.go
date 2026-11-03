// Command drivebackup mengunggah dump harian ke folder backup di Google Drive, lalu keluar.
// Container magu-drive-backup menjalankannya tiap jam; lihat deploy/deploy.sh.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"magu/internal/drive"
)

func main() {
	client := drive.NewClient(drive.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		RefreshToken: os.Getenv("GOOGLE_REFRESH_TOKEN"),
		RootFolder:   os.Getenv("DRIVE_ROOT_FOLDER_ID"),
		// Dump bisa jauh lebih besar daripada laporan, dan internet toko bisa pelan.
		Timeout: 15 * time.Minute,
	})
	if client == nil {
		slog.Info("google drive belum diatur; backup hanya tersimpan di server")
		return
	}
	dir := os.Getenv("BACKUP_DIR")
	if dir == "" {
		dir = "/backups"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Minute)
	defer cancel()

	uploaded, err := client.PushBackups(ctx, dir, time.Now())
	if err != nil {
		slog.Error("backup ke drive gagal", "uploaded", uploaded, "error", err)
		os.Exit(1)
	}
	if len(uploaded) > 0 {
		slog.Info("backup terkirim ke drive", "uploaded", uploaded)
	}
}
