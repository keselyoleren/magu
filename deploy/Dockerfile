FROM alpine:3.20
COPY server adduser drivebackup goose /usr/local/bin/
COPY migrations /migrations
ENV TZ=Asia/Jakarta
# Migrasi dijalankan setiap kali container mulai; diulang sampai Postgres siap menerima koneksi.
CMD ["sh", "-c", "until goose -dir /migrations postgres \"$DATABASE_URL\" up; do sleep 2; done; exec server"]
