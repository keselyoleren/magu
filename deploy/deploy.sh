#!/bin/sh
# Memasang Magu di server Linux yang punya Docker, tanpa sudo: container Postgres, Redis, aplikasi,
# Caddy (HTTPS), dan backup, dalam satu jaringan Docker sendiri. Aman dijalankan berulang; data di volume magu_pgdata tidak disentuh.
#
#   make deploy host=user@192.168.1.65        (ARCH=amd64 untuk server Intel/AMD)
set -eu

HOST=${1:?pakai: deploy.sh user@host}
PORT=${MAGU_PORT:-8090}
# Alamat yang diketik kasir di browser; sertifikat HTTPS dibuat untuk alamat ini.
HTTPS_HOST=${MAGU_HTTPS_HOST:-${HOST#*@}}
HERE=$(cd "$(dirname "$0")" && pwd)

ssh "$HOST" 'mkdir -p ~/magu/app ~/magu/backups'
rsync -az --delete "$HERE/out/" "$HERE/Dockerfile" "$HOST:magu/app/"
rsync -az --delete "$HERE/../backend/migrations" "$HOST:magu/app/"

ssh "$HOST" PORT="$PORT" HTTPS_HOST="$HTTPS_HOST" sh -s <<'REMOTE'
set -eu
cd ~/magu

if [ ! -f .env ]; then
  PG_PASSWORD=$(head -c 24 /dev/urandom | od -An -tx1 | tr -d ' \n')
  umask 077
  cat > .env <<ENV
POSTGRES_PASSWORD=$PG_PASSWORD
DATABASE_URL=postgres://magu:$PG_PASSWORD@magu-postgres:5432/magu?sslmode=disable&timezone=Asia/Jakarta
REDIS_URL=redis://magu-redis:6379/0
LISTEN_ADDR=0.0.0.0:8080
STORE_NAME=Toko
STORE_CODE=TK
RECEIPT_WIDTH=32
PRINTER_TARGET=
MIDTRANS_BASE_URL=https://api.sandbox.midtrans.com
MIDTRANS_SERVER_KEY=
GOOGLE_CLIENT_ID=
GOOGLE_CLIENT_SECRET=
GOOGLE_REFRESH_TOKEN=
DRIVE_ROOT_FOLDER_ID=
ENV
fi
PG_PASSWORD=$(grep '^POSTGRES_PASSWORD=' .env | cut -d= -f2)

docker network inspect magu >/dev/null 2>&1 || docker network create magu >/dev/null
running() { docker ps -a --format '{{.Names}}' | grep -qx "$1"; }

running magu-postgres || docker run -d --name magu-postgres --network magu --restart unless-stopped \
  -e POSTGRES_USER=magu -e POSTGRES_PASSWORD="$PG_PASSWORD" -e POSTGRES_DB=magu -e TZ=Asia/Jakarta \
  -v magu_pgdata:/var/lib/postgresql/data postgres:17 >/dev/null
running magu-redis || docker run -d --name magu-redis --network magu --restart unless-stopped \
  redis:7-alpine redis-server --maxmemory 64mb --maxmemory-policy allkeys-lru >/dev/null

docker build -q -t magu-app:latest app >/dev/null
docker rm -f magu-app >/dev/null 2>&1 || true
# Printer USB: tambahkan --device sesuai PRINTER_TARGET bila perangkatnya ada.
DEVICE=$(grep '^PRINTER_TARGET=/dev/' .env | cut -d= -f2 || true)
docker run -d --name magu-app --network magu --restart unless-stopped -p "$PORT:8080" --env-file .env \
  ${DEVICE:+--device "$DEVICE"} magu-app:latest >/dev/null

# HTTPS di jaringan lokal: browser hanya memberi akses ke printer USB/serial di halaman https.
# Caddy membuat sertifikatnya sendiri; sertifikat akarnya dipasang sekali di tiap komputer kasir
# dan bisa diunduh dari http://<server>:8091/magu-root-ca.crt (hanya berkas itu yang dilayani).
cat > Caddyfile <<CADDY
{
	# Browser tidak mengirim nama host (SNI) saat alamatnya berupa IP; tanpa ini Caddy tidak tahu sertifikat mana yang dipakai.
	default_sni $HTTPS_HOST
}

https://$HTTPS_HOST {
	tls internal
	reverse_proxy magu-app:8080
}

http://:8091 {
	@ca path /magu-root-ca.crt
	handle @ca {
		root * /data/caddy/pki/authorities/local
		rewrite * /root.crt
		file_server
	}
	handle {
		respond 404
	}
}
CADDY
docker rm -f magu-caddy >/dev/null 2>&1 || true
docker run -d --name magu-caddy --network magu --restart unless-stopped -p 443:443 -p 8091:8091 \
  -v "$HOME/magu/Caddyfile:/etc/caddy/Caddyfile:ro" -v magu_caddy:/data caddy:2-alpine >/dev/null

# Backup tiap malam pukul 23.00 oleh container kecil, supaya tidak bergantung pada cron atau sudo di server;
# dump disimpan di ~/magu/backups, dan yang lebih tua dari 30 hari dibuang. Saat mulai, dump langsung dibuat
# bila tidak ada yang berumur kurang dari 24 jam, karena jam 23.00 terlewat kalau server sedang mati.
# Setelah server menyala ulang container ini start bersama Postgres, jadi tunggu sampai siap. Dump ditulis ke
# berkas sementara supaya kegagalan tidak meninggalkan berkas kosong atau menimpa dump bagus di tanggal yang sama.
docker rm -f magu-backup >/dev/null 2>&1 || true
docker run -d --name magu-backup --network magu --restart unless-stopped --memory 256m --cpus 0.5 \
  --user "$(id -u):$(id -g)" -e PGPASSWORD="$PG_PASSWORD" -e TZ=Asia/Jakarta -v "$HOME/magu/backups:/backups" \
  postgres:17 sh -c '
  dump() {
    until pg_isready -q -h magu-postgres -U magu; do sleep 10; done
    f=/backups/magu-$(date +%F).dump
    if pg_dump -h magu-postgres -U magu -Fc magu > "$f.tmp" && mv "$f.tmp" "$f"; then
      find /backups -name "magu-*.dump" -mtime +30 -delete
    else
      rm -f "$f.tmp"
      return 1
    fi
  }
  [ -n "$(find /backups -name "magu-20*.dump" -size +0 -mmin -1440)" ] || until dump; do sleep 300; done
  while true; do
    now=$(date +%s)
    next=$(date -d 23:00 +%s)
    [ "$next" -gt "$now" ] || next=$((next + 86400))
    sleep $((next - now))
    until dump; do sleep 300; done
  done' >/dev/null

# Salinan ke luar server: dump harian di atas diunggah ke Google Drive. Tanpa kredensial Google, proses ini tidak melakukan apa-apa.
docker rm -f magu-drive-backup >/dev/null 2>&1 || true
# Server dipakai bersama banyak layanan lain, jadi CPU dan memorinya dibatasi; dump dibaca utuh ke memori saat diunggah.
docker run -d --name magu-drive-backup --network magu --restart unless-stopped --env-file .env --memory 256m --cpus 0.25 \
  -v "$HOME/magu/backups:/backups:ro" magu-app:latest sh -c 'while true; do drivebackup; sleep 3600; done' >/dev/null

for i in $(seq 1 60); do
  if wget -qO- "http://127.0.0.1:$PORT/healthz" 2>/dev/null | grep -q ok; then
    echo "Magu berjalan di https://$HTTPS_HOST (dan http://$HTTPS_HOST:$PORT)"
    exit 0
  fi
  sleep 2
done
echo "Aplikasi belum menjawab; lihat: docker logs magu-app" >&2
exit 1
REMOTE
