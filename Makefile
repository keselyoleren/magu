include .env
export

.PHONY: up migrate dev web test check adduser drive-auth build deploy

up:
	docker compose up -d

migrate:
	goose -dir backend/migrations postgres "$(DATABASE_URL)" up

dev:
	cd backend && go run ./cmd/server

web:
	cd web && npm run dev

# make adduser username=admin password=rahasia name="Pemilik Toko" role=admin
adduser:
	cd backend && go run ./cmd/adduser -username "$(username)" -password "$(password)" -name "$(name)" -role "$(or $(role),cashier)"

# Sekali saja, di komputer yang punya browser. Lihat docs/deployment.md bagian Google Drive.
drive-auth:
	cd backend && go run ./cmd/driveauth

# Frontend di-build lalu ditanam ke binary; hasilnya satu berkas server untuk Linux (ARCH = arm64 atau amd64).
ARCH ?= arm64
build:
	cd web && npm run build
	rm -rf backend/web/dist && cp -r web/dist backend/web/dist && touch backend/web/dist/.gitkeep
	mkdir -p deploy/out
	cd backend && GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 go build -tags timetzdata -trimpath -o ../deploy/out/ ./cmd/server ./cmd/adduser ./cmd/drivebackup
	GOOS=linux GOARCH=$(ARCH) CGO_ENABLED=0 GOBIN= go install github.com/pressly/goose/v3/cmd/goose@latest
	cp "$$(go env GOPATH)/bin/linux_$(ARCH)/goose" deploy/out/goose
	rm -rf backend/web/dist && mkdir -p backend/web/dist && touch backend/web/dist/.gitkeep

# make deploy host=user@192.168.1.65
deploy: build
	./deploy/deploy.sh $(host)

# Test yang menyentuh database berjalan di database terpisah (magu_test) yang dibuat ulang setiap kali.
test:
	docker compose exec -T postgres psql -U magu -q -c "DROP DATABASE IF EXISTS magu_test" -c "CREATE DATABASE magu_test"
	goose -dir backend/migrations postgres "$(TEST_DATABASE_URL)" up
	cd backend && go vet ./... && go test -p 1 ./...

check:
	cd web && npm run check && npm run build
