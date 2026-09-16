# AI Empire: Start Here

This page explains what exists **right now** (V1) and how the parts fit together. Read it first, then go deeper:

- [database.md](database.md): every table and column, and who writes it
- [apps.md](apps.md): the three programs, their config, API, and commands

For what's planned next, see [ROADMAP.md](../ROADMAP.md). For the full vision, see [AI_SOFTWARE_DEV_EMPIRE.md](../AI_SOFTWARE_DEV_EMPIRE.md).

---

## 1. What you have, in one picture

```text
   YOU (owner)
      │  empire CLI  (bin/empire)
      │  HTTP + owner token
      ▼
┌──────────────────────────┐        ┌───────────────────────────┐
│  CONTROL PLANE           │  SQL   │  PostgreSQL (Docker)      │
│  bin/controlplane :8787  │◄──────►│  localhost:5433 / empire  │
│  - the only thing that   │        │  the single source of     │
│    touches the database  │        │  truth for all state      │
│  - enforces rules/policy │        └───────────────────────────┘
│  - builds agent context ◄├──── reads ── knowledge/  (Markdown)
└────────────▲─────────────┘
             │  HTTP + worker token
             │  (claim, report, ask permission)
┌────────────┴─────────────┐        ┌───────────────────────────┐
│  WORKER                  │  git   │  Repositories (remotes)   │
│  bin/worker              │◄──────►│  main  +  ai/task-N       │
│  - clones each repo      │        │  (one per project repo)   │
│                          │        └───────────────────────────┘
│  - one worktree per task │
│  - runs the agent        │──► claude -p   (edits files only)
│  - runs tests, commits,  │
│    pushes, merges        │
└──────────────────────────┘
```

**The three programs:**

| Program | Who uses it | What it does |
|---------|-------------|--------------|
| `controlplane` | Everyone talks to it | The "AI Dev OS". Stores all state, enforces the task state machine and approval rules, writes the audit log. |
| `worker` | Runs in the background | Picks up tasks, runs Claude Code in an isolated git worktree, runs tests, pushes branches, merges after approval. |
| `empire` | You | A command-line remote control for the control plane. |

**The supporting pieces:**

| Piece | What it is |
|-------|------------|
| PostgreSQL | Runs in Docker (`make up`). Holds clients, projects, repositories, tasks, workers, approvals, runs, and the audit log. |
| `migrate` | A one-shot Docker container (`make migrate`) that applies `migrations/*.sql`. |
| `knowledge/` | Markdown files. Global, stack, client, and project knowledge that gets fed to the agent. |
| `workspaces/` | The worker's scratch area: repo clones, task worktrees, agent logs. Git ignores it and it's safe to delete. |

---

## 2. Key ideas (glossary)

| Term | Meaning |
|------|---------|
| **Client** | Optional. Who the work is for, and the confidentiality boundary: its knowledge only reaches its own projects. Has a default autonomy level. |
| **Project** | A product (e.g. Guest Management). Belongs to at most one client, has an autonomy level, and has one or more repositories. Project docs are shared by all its repositories. |
| **Repository** | One git repo of a project (e.g. `backend`, `frontend`), with its own URL, protected branch, **stack** (`go`, `node`) and test command. |
| **Task** | One unit of work in **one repository** of a project ("add /health endpoint"). Has a **status** and a **stage**. Can wait for other tasks (`-after`), even in other projects of the same client. |
| **Status** | Where the task is in its lifecycle: `PENDING`, `RUNNING`, `WAITING_FOR_HUMAN`, `COMPLETED`, … (9 in total). |
| **Stage** | What the worker should do next time it picks the task up: `implement` (write code) or `merge` (merge the approved code). |
| **Worker** | A process that executes tasks. It registers itself by name and sends a heartbeat every 10s. |
| **Agent run** | One execution of Claude Code for a task, with tokens, cost, and a log file. |
| **Action** | Something a worker wants to do, e.g. `push_branch` or `merge_protected`. Each action has a risk level. |
| **Autonomy level** | A per-project setting (`high` / `medium` / `conservative`) that decides which medium-risk actions need a human. |
| **Gate / approval request** | A question for you: "may the worker do X?" Created when policy says an action needs a human. |
| **Decision** | Your answer to a gate: approve, request changes (with a comment), or reject. |
| **Audit log** | An append-only history of everything that happened, and who did it. |
| **Context bundle** | `.empire-context.md`: the knowledge files the agent is allowed to see for this task. |

### How work is organized

```text
client (optional)          acme                              (none = personal)
  └── project              ├── guest                          └── go-sdk
        └── repository     │     ├── backend   (go)                 └── go-sdk (go)
              └── task     │     └── frontend  (node)
                           └── shop
                                 └── app       (go)
```

What knowledge a task gets (built by the control plane, in this order):

```text
knowledge/global/                      always
knowledge/stacks/<repository stack>/   frontend task → stacks/node, never stacks/go
knowledge/clients/<project's client>/  only for that client's projects
knowledge/projects/<project>/<doc>     only the docs listed on the task (-doc)
```

Repository-specific knowledge lives in the repository itself (README, CLAUDE.md, docs/); the agent reads it in its worktree.

A feature that spans repositories is one task per repository, chained with `-after`. You approve each repository's merge separately, and a task only starts once the tasks it depends on are merged. A chain may cross projects, but never clients.

---

## 3. The life of one task

This is what happened in the real demo run, step by step, with the database rows each step touches.

```text
 YOU                         CONTROL PLANE (DB)                         WORKER
 ───                         ──────────────────                         ──────
 empire task create ───────► tasks: new row, status=PENDING
                             stage=implement
                                                          ◄──────────── claim (every 5s)
                             tasks: status=ASSIGNED, worker_id=1,
                             attempts=1
                                                          ◄──────────── transition RUNNING
                                                                        clone/fetch repo
                                                                        worktree on ai/task-1
                                                          ◄──────────── GET context
                             reads knowledge/ ─────────────────────────► .empire-context.md
                                                          ◄──────────── start run
                             agent_runs: new row
                                                                        claude -p … (edits code)
                                                          ◄──────────── finish run
                             agent_runs: tokens, cost, exit
                                                          ◄──────────── transition TESTING
                                                                        go test ./...  ✔
                                                                        git commit
                                                          ◄──────────── authorize push_branch
                             policy: low risk → allowed
                                                                        git push ai/task-1
                                                          ◄──────────── authorize merge_protected
                             policy: high risk → needs human
                             approval_requests: new row (PENDING_APPROVAL,
                               subject_version = commit sha)
                             tasks: status=WAITING_FOR_HUMAN,
                               stage=merge, worker_id=NULL
                                                                        (worker is free again)
 empire approvals ─────────► lists the gate: agent summary + diffstat
 empire approve 1 ─────────► approval_decisions: new row
                             approval_requests: APPROVED
                             tasks: status=PENDING (stage stays merge)
                                                          ◄──────────── claim
                             tasks: ASSIGNED → RUNNING
                                                          ◄──────────── authorize merge_protected
                             finds APPROVED request → allowed,
                             returns the approved sha
                                                                        merge exactly that sha into main
                                                                        go test ./...  ✔
                                                                        git push main
                                                          ◄──────────── transition COMPLETED
                             tasks: COMPLETED
```

**Every arrow into the control plane also writes an `audit_log` row.** Run `empire audit -target task:1` to see them.

### The other two answers you can give

- **`empire request-changes 1 -m "also log requests"`.** The task goes back to `PENDING` with `stage=implement`, and your comment is stored in `tasks.feedback`. The agent gets the comment in its prompt and continues on the same `ai/task-1` branch. Then a new gate opens.
- **`empire reject 1`.** The task becomes `CANCELLED`.

### When things go wrong

| Situation | What happens |
|-----------|--------------|
| Agent or tests fail | The worker marks the task `FAILED` with `last_error`. Run `empire task retry N` to try again. |
| Worker process dies and restarts | On re-register, the control plane puts that worker's in-flight tasks back to `PENDING`. |
| Worker dies and stays dead | After `EMPIRE_STALE_AFTER` (60s) without a heartbeat, the worker becomes `offline` and its tasks are requeued. |
| A task keeps crashing | After 3 claims (`attempts`), a requeue marks it `FAILED` instead. |
| You cancel a running task | `empire task cancel N`. The worker learns on its next heartbeat and stops the agent. |
| Control plane restarts | Nothing is lost; all state is in Postgres. |

---

## 4. Who is allowed to do what

| | You (owner token) | Worker (worker token) | Agent (Claude) |
|---|---|---|---|
| Create clients / projects / repositories / tasks | ✅ | ❌ | ❌ |
| Move a project to another client | ✅ | ❌ | ❌ |
| Cancel / retry tasks | ✅ | ❌ | ❌ |
| Approve / reject gates | ✅ | ❌ | ❌ |
| Claim tasks, report status | ❌ | ✅ (only tasks it holds) | ❌ |
| Mark a task `COMPLETED` | ❌ | Only in the `merge` stage | ❌ |
| Edit files in the worktree | n/a | n/a | ✅ |
| Run shell / git commands | n/a | ✅ | ❌ (`acceptEdits` mode) |
| Call the control plane | ✅ | ✅ | ❌ (tokens are stripped from its env) |

Risk levels (from [internal/policy/policy.go](../internal/policy/policy.go)):

| Risk | Actions | Needs a human? |
|------|---------|----------------|
| Low | read_source, create_worktree, modify_code, run_tests, commit, push_branch, format_code, static_analysis, generate_docs, refactor | Never |
| Medium | schema_change, api_change, add_dependency, auth_change, infra_config, architecture_change | Depends on project autonomy (below) |
| High | merge_protected, deploy_production, destructive_migration, destructive_infra, security_policy_change, global_knowledge_promotion, permission_escalation | **Always** |
| Unknown | anything else | **Always** (fail closed) |

| Autonomy | Medium-risk actions allowed without a human |
|----------|---------------------------------------------|
| `high` | all of them |
| `medium` | add_dependency, infra_config |
| `conservative` (default) | none |

> **V1 note:** the worker currently asks about only two actions: `push_branch` and `merge_protected`. Medium-risk actions exist in policy, but nothing detects them yet (for example, noticing that the agent added a dependency).

---

## 5. Everyday commands

### Windows (PowerShell): do this first in every new terminal

`empire` isn't on your PATH, and PowerShell doesn't read `.env`. So in every new terminal, from the workspace root, run:

```powershell
. .\env.ps1        # dot + space + path: loads .env and adds bin\ to PATH
```

After that, `empire …`, `controlplane`, and `worker` work directly in that terminal. If it says `bin\empire.exe not found`, build first with `go build -o bin/ ./cmd/...`.

A typical session uses three terminals, each started with `. .\env.ps1`:

| Terminal | Command |
|----------|---------|
| 1 | `controlplane` (after Docker Desktop is running and `make up` has been run) |
| 2 | `worker` (set `$env:EMPIRE_AGENT="fake"` first for a free dry run) |
| 3 | `empire task list`, `empire approvals`, … |

Troubleshooting:

| Symptom | Fix |
|---------|-----|
| `empire : The term 'empire' is not recognized` | You skipped `. .\env.ps1` in this terminal, or `bin\` isn't built |
| `error: 401: unauthorized` | Token not loaded: run `. .\env.ps1` |
| `connectex: No connection could be made` | The control plane isn't running (terminal 1) |
| Control plane: `database: … (is make up running?)` | Start Docker Desktop, then run `make up` |
| `env.ps1 cannot be loaded because running scripts is disabled` | Run `Set-ExecutionPolicy -Scope CurrentUser RemoteSigned` once |
| Ran `.\env.ps1` without the leading dot, and nothing changed | The dot matters: `. .\env.ps1` keeps the variables in your terminal |

### Commands (any OS)

```sh
make up              # start Postgres
make migrate         # apply DB migrations
make build           # build bin/controlplane, bin/worker, bin/empire
make run-cp          # terminal 1
make run-worker      # terminal 2 (EMPIRE_AGENT=fake for a free dry run)

empire client list                  # clients
empire project list                 # projects, their client and repositories
empire repo list -project guest     # one project's repositories
empire task list                    # tasks, shown as project/repo
empire workers                      # who is alive
empire approvals                    # what needs you
empire task get 1                   # full detail: runs, cost, approvals
empire audit -target task:1         # full history

make test            # all tests (end-to-end test uses a separate empire_test DB)
make down            # stop Postgres (data is kept in a Docker volume)
```

Reset everything: `make down && docker volume rm aiempire_pgdata && rm -rf workspaces`, then `make up && make migrate`.

---

## 6. Repository map

```text
AI Empire/
├── AI_SOFTWARE_DEV_EMPIRE.md   vision / requirements
├── ROADMAP.md                  plan with checkboxes
├── README.md                   quickstart
├── docs/                       ← you are here
├── Makefile                    up, migrate, test, build, run-*
├── docker-compose.yml          postgres + migrate
├── .env / .env.example         config and dev tokens
├── env.ps1                     Windows: `. .\env.ps1` loads .env + adds bin\ to PATH
├── migrations/                 database schema (SQL)
├── cmd/
│   ├── controlplane/           main() for the control plane
│   ├── worker/                 main() for the worker
│   └── empire/                 the CLI
├── internal/
│   ├── api/                    JSON shapes shared by all three programs
│   ├── client/                 HTTP client (used by worker + CLI)
│   ├── controlplane/           HTTP handlers + all SQL
│   ├── worker/                 claim loop, git, agents, e2e test
│   ├── policy/                 risk levels, autonomy presets, "who may approve"
│   ├── task/                   task state machine
│   └── knowledge/              context bundle builder
├── knowledge/                  Markdown knowledge (open in Obsidian)
├── workspaces/                 worker scratch (git-ignored)
└── bin/                        built binaries (git-ignored)
```

## 7. Known limitations (V1)

- All workers share one token, and a worker's identity (`X-Worker-ID`) is self-declared.
- A worker runs one task at a time, and there's no per-project lock for running tasks in parallel on one machine.
- Cancelling a task closes its open gates as `REJECTED` without writing an `approval_decisions` row.
- Clients, projects and repositories can be created and edited (projects can be moved), but slugs/names cannot be renamed and nothing can be deleted yet.
- A feature spanning repositories gets one merge gate per repository. A combined all-or-nothing approval is deferred to V3.
- No web UI, notifications, or Hermes yet (that's V2).
- No document contracts, workflow engine, or knowledge graph yet (that's V3).
