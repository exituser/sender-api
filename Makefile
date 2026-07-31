.PHONY: build run worker dev dev-down test clean migrate-up migrate-down lint tidy backup restore

build:
	go build -o bin/api ./cmd/api
	go build -o bin/worker ./cmd/worker

run: build
	./bin/api

worker: build
	./bin/worker

dev:
	docker compose -f docker-compose.dev.yml up

dev-down:
	docker compose -f docker-compose.dev.yml down

test:
	go test ./cmd/... ./internal/... ./pkg/...

clean:
	rm -rf bin/
	rm -rf tmp/

migrate-up:
	migrate -path migrations -database "$(DATABASE_URL)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down

lint:
	golangci-lint run

tidy:
	go mod tidy

backup:
	./scripts/backup-db.sh

restore:
	./scripts/restore-db.sh "$(BACKUP_FILE)"
