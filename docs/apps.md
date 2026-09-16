# Apps Reference

The three programs in V1: what each one does, how to configure it, and how they talk. For the big picture, read [README.md](README.md) first.

| App | Source | Binary | Role |
|-----|--------|--------|------|
| Control plane | [cmd/controlplane](../cmd/controlplane/main.go) → [internal/controlplane](../internal/controlplane/) | `bin/controlplane` | HTTP API, owns the database, enforces rules |
| Worker | [cmd/worker](../cmd/worker/main.go) → [internal/worker](../internal/worker/) | `bin/worker` | Executes tasks with git + Claude Code |
| CLI | [cmd/empire](../cmd/empire/main.go) | `bin/empire` | Your remote control |

Build all three with `make build`. All config comes from environment variables. `make run-*` loads `.env` automatically, but the binaries don't, so load `.env` into your shell before running them directly. On Windows, run `. .\env.ps1`.

---

## 1. Control plane

### What it does

- Serves the HTTP API on `CP_ADDR` (default `:8080`).
- Is the **only** program with database access.
- Enforces:
  - **Authentication:** owner token vs worker token.
  - **Task state machine:** [internal/task/state.go](../internal/task/state.go).
  - **Policy:** which actions need a human ([internal/policy/policy.go](../internal/policy/policy.go)).
  - **Approval rules:** only a human decides, and never the requester.
  - **Task ownership:** a worker can only touch tasks it currently holds.
- Builds the agent's context bundle from `knowledge/` ([internal/knowledge/context.go](../internal/knowledge/context.go)).
- Writes an `audit_log` row for every change, in the same transaction.
- Runs the **reaper** in the background every `EMPIRE_STALE_AFTER / 2`. It marks silent workers `offline` and requeues their tasks.

### Configuration

| Variable | Default | Meaning |
|----------|---------|---------|
| `DATABASE_URL` | `postgres://empire:empire@localhost:5433/empire?sslmode=disable` | Postgres connection |
| `EMPIRE_OWNER_TOKEN` | *(required)* | Bearer token for you |
| `EMPIRE_WORKER_TOKEN` | *(required, must differ from the owner token)* | Bearer token for workers |
| `CP_ADDR` | `:8080` | Listen address |
| `EMPIRE_KNOWLEDGE_DIR` | `knowledge` | Root of the knowledge repo |
| `EMPIRE_STALE_AFTER` | `60s` | A worker with no heartbeat for this long is considered dead |

It refuses to start if the tokens are missing or the database is unreachable.

### Code layout

| File | Contents |
|------|----------|
| [server.go](../internal/controlplane/server.go) | Routes, auth, error mapping, JSON helpers, transaction and audit helpers |
| [tasks.go](../internal/controlplane/tasks.go) | Projects, tasks, cancel/retry, `move()` (the single status-change function) |
| [workers.go](../internal/controlplane/workers.go) | Worker protocol: register, heartbeat, claim, transition, authorize, context, runs. Also the reaper |
| [approvals.go](../internal/controlplane/approvals.go) | List approvals, `decide()` |

### HTTP API

Every request needs `Authorization: Bearer <token>`. Worker calls also send `X-Worker-ID: <id>`. Errors come back as `{"error": "..."}`.

| Status code | Meaning |
|-------------|---------|
| 400 | Bad input |
| 401 | Bad token |
| 403 | Right token, wrong role, or not allowed |
| 404 | Not found |
| 409 | Illegal state change, or already decided |

**Owner endpoints**

| Method & path | Body | Does |
|---------------|------|------|
| `POST /projects` | `{slug, name, repo_url, stack, default_branch?, autonomy_level?, test_command?}` | Create a project |
| `POST /tasks` | `{project (slug), title, description?, required_capabilities?, context_docs?, depends_on?}` | Create a task. Checks that `context_docs` exist |
| `POST /tasks/{id}/cancel` | none | → `CANCELLED`, and closes open gates |
| `POST /tasks/{id}/retry` | none | `FAILED` → `PENDING`, resets attempts |
| `POST /approvals/{id}/approve` | `{comment?}` | Task → `PENDING` (resumes at its stage) |
| `POST /approvals/{id}/request-changes` | `{comment}` (required) | Task → `PENDING`, `stage=implement`, feedback saved |
| `POST /approvals/{id}/reject` | `{comment?}` | Task → `CANCELLED` |
| `GET /audit?target=task:1` | none | Last 200 audit rows (all, or for one target) |

**Read endpoints (owner or worker)**

| Method & path | Returns |
|---------------|---------|
| `GET /projects`, `GET /projects/{id}` | Projects |
| `GET /tasks?status=&project_id=` | Up to 500 tasks, newest first |
| `GET /tasks/{id}` | `{task, runs, approvals}` |
| `GET /approvals?status=`, `GET /approvals/{id}` | Approval requests |
| `GET /workers` | Workers |
| `GET /healthz` | `ok` (no auth) |

**Worker endpoints.** Task endpoints only work on a task the calling worker currently holds.

| Method & path | Body | Does |
|---------------|------|------|
| `POST /workers/register` | `{name, capabilities}` | Upsert by name, return the worker. **Requeues anything it held before** |
| `POST /workers/{id}/heartbeat` | `{task_id?}` | Refresh liveness. Replies `{cancel: true}` if the task is no longer the worker's |
| `POST /workers/{id}/claim` | none | Next matching `PENDING` task (`{task, project}`), or `204` if none |
| `POST /tasks/{id}/transition` | `{to, error?}` | Allowed `to`: `RUNNING`, `TESTING`, `REVIEWING`, `COMPLETED` (merge stage only), `FAILED`, `PENDING` |
| `POST /tasks/{id}/authorize` | `{action, stage, summary?, subject_version?}` | Policy check. Replies `{allowed, approval_id?, subject_version?}`. If a human is needed, opens a gate and parks the task |
| `GET /tasks/{id}/context` | none | `{files, content}`, the context bundle |
| `POST /tasks/{id}/runs` | `{model, context_files}` | Start an agent run, return `{id}` |
| `POST /runs/{id}/finish` | `{exit_status, log_path, tokens, cost_usd}` | Close the run |

**How claiming works:** a single SQL query picks the lowest-id `PENDING` task that meets all of these conditions:
- its required capabilities ⊆ the worker's capabilities
- all its dependencies are `COMPLETED`
- the worker is `online`

It uses `FOR UPDATE SKIP LOCKED`, so two workers can never claim the same task.

---

## 2. Worker

### What it does

On start: **register**, then loop forever. The loop is **claim** a task → execute it → repeat, with a poll every 5s when idle. A separate **heartbeat** runs every 10s.

```text
claim ──► RUNNING ──► ensure clone  workspaces/<slug>/_base   (git clone --no-checkout, then fetch)
                          │
            ┌─────────────┴──────────────┐
      stage=implement                stage=merge
            │                              │
  worktree workspaces/<slug>/task-N   authorize merge_protected
  on branch ai/task-N                   (must already be APPROVED → gives sha)
  (continues the pushed branch          worktree (detached) at origin/main
   if it exists)                        git merge --no-ff <approved sha>
            │                           TESTING: test_command
  GET context → .empire-context.md      git push origin HEAD:main
  start run → claude -p → finish run    COMPLETED
            │
  TESTING: test_command
  git add -A, commit "title\n\nTask: N"
  (fails if nothing changed)
  authorize push_branch → git push ai/task-N
  authorize merge_protected
     → gate opened, task parked (WAITING_FOR_HUMAN)
            │
  remove worktree
```

Any error → `FAILED` with the error text (last 4000 chars) in `last_error`.

### Configuration

| Variable | Default | Meaning |
|----------|---------|---------|
| `EMPIRE_CP_URL` | `http://localhost:8080` | Control plane address |
| `EMPIRE_WORKER_TOKEN` | *(required)* | Must match the control plane's |
| `EMPIRE_WORKER_NAME` | hostname | Identity. Reusing a name reuses the worker row |
| `EMPIRE_WORKER_CAPS` | `go` | Comma-separated capabilities |
| `EMPIRE_WORKSPACES` | `workspaces` | Where clones, worktrees and logs go |
| `EMPIRE_AGENT` | `claude` | `claude` = real Claude Code. `fake` = appends a line to `empire-fake-agent.txt` (free, for testing) |
| `EMPIRE_AGENT_MODEL` | *(empty)* | Passed as `claude --model` |

### Files on disk

```text
workspaces/
├── <slug>/
│   ├── _base/           one clone per project (.git/info/exclude hides .empire-context.md)
│   └── task-7/          worktree while task 7 runs (deleted afterwards)
└── logs/
    └── task-7-run-3.log full agent output (JSON from claude -p)
```

### The agent (Claude Code)

It's run as `claude -p --output-format json --permission-mode acceptEdits [--model M]`:
- **Where it runs:** the task worktree is its working directory.
- **Input:** the prompt arrives on stdin.
- **What it may do:** edit files. It can't run shell commands (`acceptEdits`); tests and git are done by the worker.
- **Environment:** every `EMPIRE_*` variable and `DATABASE_URL` are removed, so it can't call the control plane.
- **What it's told:** the task, your feedback if any, to read `.empire-context.md`, not to touch git, and to flag problems with approved requirements instead of silently changing them. The prompt is built in `prompt()` in [worker.go](../internal/worker/worker.go).
- **What gets recorded:** tokens and cost are parsed from its JSON output.

To add another agent, implement the `Agent` interface in [agent.go](../internal/worker/agent.go) (`Name()` and `Run(ctx, dir, prompt, log)`) and add it to the switch in [cmd/worker/main.go](../cmd/worker/main.go).

### Code layout

| File | Contents |
|------|----------|
| [worker.go](../internal/worker/worker.go) | Register, heartbeat, `Step` (claim + execute), implement/merge stages, prompt |
| [git.go](../internal/worker/git.go) | Command runner, clone/fetch, worktree add/remove |
| [agent.go](../internal/worker/agent.go) | `Agent` interface, `ClaudeCode`, `Fake` |
| [e2e_test.go](../internal/worker/e2e_test.go) | Full V1 flow against real Postgres + git |

---

## 3. CLI (`empire`)

It uses `EMPIRE_CP_URL` (default `http://localhost:8080`) and `EMPIRE_OWNER_TOKEN`. **Flags go before positional arguments.**

On Windows, run `. .\env.ps1` in the terminal first ([env.ps1](../env.ps1)). It sets both variables and adds `bin\` to PATH.

| Command | Example |
|---------|---------|
| Create project | `empire project create -slug shop -name "Shop" -repo git@github.com:me/shop.git -stack go -autonomy medium -test "go test ./..."` |
| List projects | `empire project list` |
| Create task | `empire task create -project shop -desc "details…" -doc requirements/prd.md -after 3 Add QR validation` |
| List tasks | `empire task list` · `empire task list -status FAILED` · `empire task list -project-id 1` |
| Task detail | `empire task get 5` (JSON: task + runs + approvals) |
| Cancel / retry | `empire task cancel 5` · `empire task retry 5` |
| Pending gates | `empire approvals` (default `-status PENDING_APPROVAL`) |
| Decide | `empire approve 2 -m "ok"` · `empire request-changes 2 -m "add tests"` · `empire reject 2` |
| Workers | `empire workers` |
| Audit | `empire audit` · `empire audit -target task:5` |

`task create` flags:

| Flag | Meaning |
|------|---------|
| `-project` | Project slug (required) |
| `-desc` | Description sent to the agent |
| `-doc` | Repeatable. A file under `knowledge/projects/<slug>/` to include in context |
| `-cap` | Repeatable. Required worker capability (default: the project's stack) |
| `-after` | Repeatable. A task id that must be `COMPLETED` first |

---

## 4. Shared packages

| Package | Used by | Contents |
|---------|---------|----------|
| [internal/api](../internal/api/types.go) | all | JSON structs for every request/response. Also the DB row mapping (`db` tags) |
| [internal/client](../internal/client/client.go) | worker, CLI | `Client.Do(method, path, in, out)`, which returns `*client.Error` on non-2xx responses |
| [internal/task](../internal/task/state.go) | control plane, worker | Status/stage constants, `CanTransition`, `InFlight`, `MaxAttempts = 3` |
| [internal/policy](../internal/policy/policy.go) | control plane, worker | Action names, risk sets, `Requires`, `CanDecide` |
| [internal/knowledge](../internal/knowledge/context.go) | control plane | `Resolve(root, stack, project, docs)`, which returns a `Bundle` |

### Context bundle rules

What `knowledge.Resolve` includes, in order:
1. every `*.md` under `knowledge/global/`
2. every `*.md` under `knowledge/stacks/<project stack>/`
3. only the task's `context_docs`, from `knowledge/projects/<slug>/`

`README.md` files are skipped (they're folder navigation). The function never reads other stacks or other projects. Doc paths that try to escape the project folder (`..`, absolute paths, symlinks) are rejected by `os.OpenInRoot`.

---

## 5. Tests

| Test | What it proves |
|------|----------------|
| [task/state_test.go](../internal/task/state_test.go) | Legal and illegal transitions |
| [policy/policy_test.go](../internal/policy/policy_test.go) | Presets, "high risk always asks", fail-closed behavior, no self-approval, no AI approval |
| [knowledge/context_test.go](../internal/knowledge/context_test.go) | No leakage across stacks or projects, path-escape rejection |
| [worker/e2e_test.go](../internal/worker/e2e_test.go) | Implement → gate → request changes → rework → approve → merge of the approved sha; restart and heartbeat-loss requeue; dependencies; cancel; audit is complete and append-only |

`make test` runs them all. The e2e test drops and recreates the `empire_test` database. It is skipped if Postgres isn't reachable.
