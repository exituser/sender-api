.PHONY: build run worker dev dev-down test clean migrate-up migrate-down migrate-check lint tidy backup restore

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
	./scripts/migrate.sh

migrate-check:
	@test -n "$${DATABASE_URL:-}" || (echo 'DATABASE_URL must be set' >&2; exit 1)
	@test "$$(psql "$$DATABASE_URL" -Atc "SELECT CASE WHEN MAX(version) = 13 AND COUNT(*) FILTER (WHERE version = 13 AND NOT dirty) = 1 AND COUNT(*) FILTER (WHERE dirty) = 0 THEN '13|f' ELSE 'invalid' END FROM public.schema_migrations")" = '13|f'

migrate-down:
	migrate -path migrations -database "$(DATABASE_URL)" down

lint:
	golangci-lint run

tidy:
	go mod tidy

backup:
	./scripts/backup-postgres.sh

restore:
	./scripts/restore-db.sh "$(BACKUP_FILE)"
