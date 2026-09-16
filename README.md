# AI Empire

Self-hosted AI software development platform. Spec: [AI_SOFTWARE_DEV_EMPIRE.md](AI_SOFTWARE_DEV_EMPIRE.md). Plan: [ROADMAP.md](ROADMAP.md).

## Requirements

Go 1.25+, Docker Desktop, GNU make.

## Quickstart

```sh
cp .env.example .env   # optional; defaults work
make up                # PostgreSQL on localhost:5433
make migrate           # apply migrations/ (golang-migrate in Docker)
make test
make run-cp            # control plane on :8080, GET /healthz
make run-worker
```

## Layout

```text
cmd/controlplane/   HTTP API + scheduler
cmd/worker/         worker binary
internal/           domain packages (added from M1.1 on)
migrations/         NNNNNN_name.up.sql / .down.sql
knowledge/          Markdown knowledge repo (open in Obsidian)
```

## Migrations

Add a pair `migrations/NNNNNN_name.up.sql` and `.down.sql`, then run `make migrate`. Use `make migrate-down` to roll back the last one.
