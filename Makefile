-include .env
export

EMPIRE_TEST_DATABASE_URL ?= postgres://$(or $(POSTGRES_USER),empire):$(or $(POSTGRES_PASSWORD),empire)@localhost:$(or $(POSTGRES_PORT),5433)/empire_test?sslmode=disable

.PHONY: up down migrate migrate-down test build run-cp run-worker

up:
	docker compose up -d --wait postgres

down:
	docker compose down

migrate:
	docker compose run --rm migrate

# Roll back the last migration
migrate-down:
	docker compose run --rm migrate down 1

# The end-to-end test uses its own empire_test database (needs `make up`).
test:
	go test ./...

build:
	go build -o bin/ ./cmd/...

run-cp:
	go run ./cmd/controlplane

run-worker:
	go run ./cmd/worker
