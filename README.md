# Magu

Aplikasi kasir (POS) dan manajemen konsinyasi berbasis web.

## Mulai dari sini

1. [docs/architecture.md](docs/architecture.md) — stack, struktur, dan keputusan yang masih terbuka
2. [docs/database.md](docs/database.md) — skema dan index
3. [docs/consignment-logic.md](docs/consignment-logic.md) — komisi, stok, pembagian hasil
4. [docs/backend-realtime.md](docs/backend-realtime.md) — checkout, webhook QRIS, WebSocket
5. [docs/frontend-hardware.md](docs/frontend-hardware.md) — printer thermal, scanner, katalog
6. [docs/deployment.md](docs/deployment.md) — pemasangan di PC Linux toko
7. [docs/roadmap.md](docs/roadmap.md) — urutan pengerjaan

Standar kode ada di [CLAUDE.md](CLAUDE.md), aturan per area ada di `.claude/skills/`.

## Menjalankan

Butuh Go 1.25+, Node 22+, Docker, dan goose (`go install github.com/pressly/goose/v3/cmd/goose@latest`).

```
cp .env.example .env     # ubah POSTGRES_PORT / REDIS_PORT bila port bawaan sudah terpakai
make up
make migrate
make adduser username=admin password=<password> name="Nama Anda" role=admin
make dev                 # terminal 1
make web                 # terminal 2, buka http://localhost:5173
```
