# Magu — POS & Konsinyasi

Aplikasi kasir web untuk toko retail yang menjual barang titipan (konsinyasi). Satu toko, puluhan ribu produk, beberapa terminal kasir.

## Stack

- **Backend**: Go + Fiber v2, pgx v5 (SQL mentah, tanpa ORM), goose untuk migrasi
- **Database**: PostgreSQL 17 — sumber kebenaran tunggal
- **Cache**: Redis — hanya sesi login dan snapshot katalog
- **Frontend**: Svelte 5 + Vite + TypeScript, SPA dengan PWA (`vite-plugin-pwa`)

## Deployment

Satu PC Linux di jaringan lokal toko menjalankan backend, Postgres, dan Redis dalam container Docker (`deploy/deploy.sh`). Kasir membukanya lewat HTTP biasa di jaringan lokal, jadi kode frontend tidak boleh bergantung pada API khusus HTTPS (mis. `crypto.randomUUID`; pakai `lib/uuid.ts`). HTTPS (Caddy, sertifikat buatan sendiri) tersedia untuk pilihan cetak lanjutan. Printer thermal tercolok di komputer kasir dengan driver biasa; struk dicetak lewat cetak browser (`lib/printer.ts`), tanpa penyiapan khusus. Scanner barcode USB bekerja sebagai keyboard. Webhook QRIS masuk lewat tunnel keluar. Jangan menambah ketergantungan pada layanan cloud selain payment gateway dan Google Drive (laporan ke member dan salinan backup; opsional, dan toko tetap jalan tanpa keduanya).

## Struktur

```
backend/
  cmd/server/main.go     wiring: config, pool, router
  internal/<fitur>/      auth, catalog, consignment, sales, qris, drive, realtime, printer
  migrations/            SQL goose, hanya maju
web/
  src/lib/               api, session, route, catalog, scanner, escpos, receipt, printer, sales
  src/routes/            satu folder per layar
docs/                    desain dan alasan di baliknya
```

## Perintah

Salin `.env.example` ke `.env` dulu. `make` membaca `.env`; jangan di-`source` dari shell (URL-nya memuat `&`).

```
make up        # Postgres + Redis (docker compose)
make migrate   # goose up
make dev       # backend di :8080
make web       # Vite di :5173, proxy /api, /healthz, /ws ke backend
make test      # go vet + go test; membuat ulang database magu_test untuk test yang menyentuh DB
make check     # svelte-check + build frontend
make adduser username=... password=... name="..." role=admin|cashier
make drive-auth  # sekali saja: hubungkan akun Google toko; lihat docs/deployment.md
make build     # frontend + binary Linux di deploy/out (ARCH=arm64|amd64)
make deploy host=user@server   # pasang ke server ber-Docker; lihat docs/deployment.md
```

## Aturan kode

Berlaku untuk setiap perubahan, di backend maupun frontend.

**Jangan over-engineering**
- Tulis solusi paling sederhana yang memenuhi kebutuhan hari ini. Jangan menambah opsi, lapisan, atau konfigurasi untuk kebutuhan yang belum ada.
- Abstraksi (interface, generic, helper, base class) baru dibuat saat sudah ada dua pemakai nyata. Satu implementasi = pakai tipe konkretnya langsung.
- Tidak ada repository pattern, DI container, event bus, atau CQRS. Handler memanggil service, service menjalankan SQL.
- Dependensi baru harus menggantikan kode yang jelas lebih banyak daripada yang ditambahkannya. Utamakan standard library.
- Ubah hanya yang diminta. Jangan merapikan, mengganti nama, atau memformat ulang kode di sekitar perubahan.

**Komentar seperlunya**
- Default: tanpa komentar. Nama yang jelas menggantikan komentar.
- Komentar hanya untuk *alasan* yang tidak terbaca dari kode: aturan bisnis, jebakan konkurensi, keanehan hardware atau gateway.
- Jangan menulis komentar yang mengulang kode, menandai bagian (`// --- handlers ---`), atau menceritakan perubahan (`// diperbaiki`, `// ditambahkan untuk X`). Riwayat ada di git.
- Jangan menambah docstring ke fungsi yang tidak kamu ubah.

**Penamaan**: identifier, nama tabel, kolom, dan path endpoint dalam bahasa Inggris. Teks yang dilihat pengguna dan pesan error dalam bahasa Indonesia.

## Aturan domain yang tidak boleh dilanggar

- Uang selalu `BIGINT`/`int64` rupiah utuh. Tidak pernah float. Komisi dalam basis poin (`2000` = 20%).
- Setiap perubahan `products.stock` menulis satu baris `stock_movements` dalam transaksi DB yang sama.
- Harga dan komisi di `transaction_items` adalah snapshot. Jangan pernah menghitung ulang dari master data.
- Stok berkurang dan hak vendor diakui hanya saat transaksi berstatus `paid`.
- Webhook pembayaran harus idempoten dan memverifikasi signature serta nominal.

## Baca sebelum mengerjakan

| Area | Dokumen | Skill |
|---|---|---|
| Gambaran & keputusan | `docs/architecture.md` | — |
| Tabel, index, migrasi | `docs/database.md` | `postgres-schema` |
| Komisi, stok, settlement | `docs/consignment-logic.md` | `consignment-rules` |
| Handler, service, SQL di Go | `docs/backend-realtime.md` | `go-backend` |
| QRIS, webhook, WebSocket | `docs/backend-realtime.md` | `payments-realtime` |
| Layar, state, katalog offline | `docs/frontend-hardware.md` | `svelte-frontend` |
| Printer thermal, scanner | `docs/frontend-hardware.md` | `pos-printing` |
| Pemasangan di PC toko | `docs/deployment.md` | — |
| Urutan pengerjaan | `docs/roadmap.md` | — |
