-- +goose Up

-- drive_folder_id NULL = folder laporan member belum dibuat.
-- drive_shared_email = alamat yang saat ini punya akses; bisa tertinggal dari email sampai sinkron berikutnya.
ALTER TABLE members
  ADD COLUMN email               TEXT,
  ADD COLUMN drive_folder_id     TEXT,
  ADD COLUMN drive_permission_id TEXT,
  ADD COLUMN drive_shared_email  TEXT;

-- drive_sent_at NULL = laporan belum dikirim ke folder member.
ALTER TABLE vendor_settlements
  ADD COLUMN drive_sent_at TIMESTAMPTZ;
