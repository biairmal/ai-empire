.PHONY: up down migrate migrate-down test run-cp run-worker

up:
	docker compose up -d --wait postgres

down:
	docker compose down

migrate:
	docker compose run --rm migrate

# Roll back the last migration
migrate-down:
	docker compose run --rm migrate down 1

test:
	go test ./...

run-cp:
	go run ./cmd/controlplane

run-worker:
	go run ./cmd/worker
