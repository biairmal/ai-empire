# AI Empire

Self-hosted AI software development platform. Spec: [AI_SOFTWARE_DEV_EMPIRE.md](AI_SOFTWARE_DEV_EMPIRE.md). Plan: [ROADMAP.md](ROADMAP.md).

**New here or lost? Read [docs/README.md](docs/README.md)** (overview), then [docs/database.md](docs/database.md) and [docs/apps.md](docs/apps.md).

**Status: V1 (M0–M1.9). V2 deferred; next is V3.** A human creates a task in one repository of a project. A worker implements it with Claude Code in an isolated git worktree and runs the tests. It pushes a branch and then stops at the merge gate. When a human approves, the worker merges the approved commit.

## Requirements

Go 1.25+, Docker Desktop, GNU make, git. For the real agent: the `claude` CLI, logged in.

## Quickstart

```sh
cp .env.example .env   # dev tokens; the Makefile loads it
make up                # PostgreSQL on localhost:5433
make migrate
make test              # unit tests + end-to-end test (own empire_test database)
make build             # bin/controlplane, bin/worker, bin/empire
```

> **Windows / PowerShell:** in every new terminal, run `. .\env.ps1` first. It loads `.env` and puts `bin\` on PATH, so `empire`, `controlplane`, and `worker` work directly. See [docs/README.md §5](docs/README.md#5-everyday-commands).

Run, each in its own terminal (the binaries read env vars, so load `.env` into your shell first, or use `make run-cp` / `make run-worker`):

```sh
make run-cp
make run-worker        # EMPIRE_AGENT=fake for a free dry run
```

Drive it with the CLI (needs `EMPIRE_OWNER_TOKEN` in the env):

```sh
# client → project → repositories  (client is optional; skip it for personal projects)
empire client create -slug acme -name "Acme" -autonomy medium
empire project create -slug guest -name "Guest Management" -client acme
empire repo add -project guest -name backend  -repo git@github.com:me/guest-be.git -stack go   -test "go test ./..."
empire repo add -project guest -name frontend -repo git@github.com:me/guest-fe.git -stack node -test "npm test"

empire task create -project guest -repo backend -desc "GET /health returns {\"status\":\"ok\"}" add /health endpoint
empire task create -project guest -repo frontend -after 1 show API health in the footer
empire task list
empire approvals                       # the merge gate: agent summary + diffstat
empire approve 1 -m "looks good"       # or: request-changes 1 -m "...", reject 1
empire task get 1                      # task + agent runs (tokens, cost, log path) + approvals
empire audit -target task:1
empire workers
```

## How a task flows

```text
PENDING → ASSIGNED → RUNNING ─ agent edits worktree (branch ai/task-N)
                     TESTING ─ project test command, commit, push branch
                     WAITING_FOR_HUMAN ─ merge_protected gate
   approve         → PENDING (stage=merge) → worker merges the approved sha, re-tests, pushes → COMPLETED
   request-changes → PENDING (stage=implement, feedback in prompt) → rework on the same branch
   reject          → CANCELLED
```

- **Authority.** Workers ask `POST /tasks/{id}/authorize` before gated actions. Policy is in [internal/policy](internal/policy/policy.go): presets per project `autonomy_level`, and high-risk actions always go to a human. Only the owner token can decide approvals, and the requester can never decide its own request.
- **Crash safety.** All state is in Postgres. A worker that re-registers, or misses heartbeats for `EMPIRE_STALE_AFTER`, has its in-flight tasks requeued, and a task fails after 3 attempts.
- **Organization.** Client (optional) → projects → repositories. Each task targets one repository. Dependencies (`-after`) can cross projects, but never clients. There is one merge gate per repository.
- **Context.** The control plane builds `.empire-context.md` from `knowledge/global`, `knowledge/stacks/<repository stack>`, `knowledge/clients/<project's client>` (if any), and the task's `-doc` files under `knowledge/projects/<slug>/`, and nothing else. The file is git-excluded.
- **Isolation.** One clone per repository, and one worktree per task, under `EMPIRE_WORKSPACES/<project>/<repo>/`. The agent runs with `--permission-mode acceptEdits` (it can edit files but not run shell commands), and `EMPIRE_*` and `DATABASE_URL` are stripped from its environment.
- **Audit.** Every mutation writes `audit_log` in the same transaction. The table is append-only (enforced by a trigger).

## Layout

```text
cmd/controlplane/        HTTP API + reaper
cmd/worker/              worker binary
cmd/empire/              owner CLI
internal/api/            shared JSON types
internal/controlplane/   handlers + SQL (modular monolith)
internal/worker/         claim loop, git worktrees, agents (claude, fake), e2e test
internal/policy/         risk classes, autonomy presets, approval rules
internal/task/           task state machine
internal/knowledge/      context resolver
migrations/              NNNNNN_name.up.sql / .down.sql (golang-migrate)
knowledge/               Markdown knowledge repo (open in Obsidian)
```

## API

Auth: `Authorization: Bearer <token>`. Worker calls also send `X-Worker-ID`.

| Owner | Worker |
|-------|--------|
| `POST /clients`, `GET /clients[/{ref}]` | `POST /workers/register` |
| `POST /projects`, `GET /projects[/{ref}]`, `PATCH /projects/{ref}`, `POST /projects/{ref}/client` | `POST /workers/{id}/heartbeat`, `POST /workers/{id}/claim` |
| `POST /projects/{ref}/repositories`, `GET /projects/{ref}/repositories`, `PATCH /projects/{ref}/repositories/{name}`, `GET /repositories` | `POST /tasks/{id}/transition`, `POST /tasks/{id}/authorize` |
| `POST /tasks`, `GET /tasks[/{id}]`, `POST /tasks/{id}/cancel`, `POST /tasks/{id}/retry` | `GET /tasks/{id}/context` |
| `GET /approvals[/{id}]`, `POST /approvals/{id}/approve` · `/request-changes` · `/reject` | `POST /tasks/{id}/runs`, `POST /runs/{id}/finish` |
| `GET /audit?target=task:1`, `GET /workers` | |
