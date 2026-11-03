-- +goose Up

-- Isi QR dari gateway disimpan supaya layar kasir bisa menampilkannya lagi setelah halaman dimuat ulang.
ALTER TABLE payments ADD COLUMN qr_string TEXT;
