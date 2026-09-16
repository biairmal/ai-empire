# Database Model

PostgreSQL 17 is the **single source of truth** for platform state (spec §31). Only the control plane connects to it. Workers and the CLI go through the HTTP API.

- Schema source: [migrations/000002_core.up.sql](../migrations/000002_core.up.sql)
- Connection (dev): `postgres://empire:empire@localhost:5433/empire`
- Open a SQL shell: `docker compose exec postgres psql -U empire -d empire`

Source code, documents and knowledge are **not** in the database. They live in git.

---

## 1. Entity relationship diagram

```text
                        ┌──────────────────────┐
                        │       projects       │
                        │──────────────────────│
                        │ id  PK               │
                        │ slug  UNIQUE         │
                        │ repo_url, stack      │
                        │ autonomy_level       │
                        └──────────┬───────────┘
                     1 ┌───────────┴────────────┐ 1
                       │                        │
                     * ▼                        ▼ *
┌──────────────────────────┐          ┌──────────────────────────┐
│          tasks           │          │    approval_requests     │
│──────────────────────────│          │──────────────────────────│
│ id  PK                   │ 1      * │ id  PK                   │
│ project_id  FK ──────────│◄─ ─ ─ ─ ─│ project_id  FK           │
│ status  (task_status)    │ (by text │ subject_type = 'task'    │
│ stage                    │  ref, no │ subject_ref  = task id   │
│ worker_id  FK ───────┐   │   FK)    │ gate, status             │
│ attempts, feedback   │   │          │ subject_version (sha)    │
└──┬───────▲───────────┼───┘          └────────────┬─────────────┘
   │ 1     │ 1         │ *                         │ 1
   │       │           │                           │
   │ *     │ *         ▼ 0..1                      ▼ *
┌──┴───────┴──────┐  ┌─────────────────────┐  ┌──────────────────────────┐
│task_dependencies│  │       workers       │  │   approval_decisions     │
│─────────────────│  │─────────────────────│  │──────────────────────────│
│ task_id    FK   │  │ id  PK              │  │ id  PK                   │
│ depends_on FK   │  │ name  UNIQUE        │  │ approval_request_id  FK  │
└─────────────────┘  │ capabilities[]      │  │ decision, comment        │
                     │ status, heartbeat   │  │ decided_by               │
                     └──────────┬──────────┘  └──────────────────────────┘
   tasks 1                      │ 1
     │                          │
     │ *                        │ *
┌────┴──────────────────────────┴──┐       ┌──────────────────────────┐
│            agent_runs            │       │        audit_log         │
│──────────────────────────────────│       │──────────────────────────│
│ id  PK                           │       │ id  PK                   │
│ task_id FK,  worker_id FK        │       │ actor_type, actor_id     │
│ model, context_files[]           │       │ action, target (text)    │
│ tokens, cost_usd, log_path       │       │ payload jsonb            │
└──────────────────────────────────┘       │ APPEND-ONLY (trigger)    │
                                           └──────────────────────────┘
                                             (no FKs: references any
                                              row as "task:1" etc.)
```

**Why approvals link to tasks by text rather than a foreign key:** approval requests are meant to cover other subjects later (PRDs and designs in V3), so they point at their subject with `subject_type` + `subject_ref` instead of a `task_id` column.

Also present: `schema_migrations`, which golang-migrate manages. It records the current schema version (2).

---

## 2. Enum types

| Type | Values |
|------|--------|
| `task_status` | `PENDING`, `ASSIGNED`, `RUNNING`, `TESTING`, `REVIEWING`, `WAITING_FOR_HUMAN`, `COMPLETED`, `FAILED`, `CANCELLED` |
| `approval_status` | `PENDING_APPROVAL`, `APPROVED`, `CHANGES_REQUESTED`, `REJECTED` |
| `autonomy_level` | `high`, `medium`, `conservative` |

`REVIEWING` exists for the AI review step planned in V3. V1 never uses it.

### Task status transitions

These are enforced in [internal/task/state.go](../internal/task/state.go). Every change goes through one function, `move()`, in [internal/controlplane/tasks.go](../internal/controlplane/tasks.go).

```text
                      ┌───────────── requeue (crash / restart / approve / changes) ──────┐
                      │                                                                  │
 PENDING ──► ASSIGNED ──► RUNNING ──► TESTING ──► WAITING_FOR_HUMAN ──► PENDING ─────────┘
    │           │           │  ▲         │                │
    │           │           │  └─────────┤                └──► CANCELLED
    │           │           │            │
    │           ▼           ▼            ▼
    │        FAILED ◄───────┴────────────┤          COMPLETED  (only from RUNNING/TESTING/
    │           │                        │                      REVIEWING, and only in stage=merge)
    │           └──► PENDING (retry)     │
    ▼                                    ▼
 CANCELLED ◄──── (owner cancel from any non-final state except FAILED)
```

Terminal states: `COMPLETED` and `CANCELLED`. `FAILED` isn't terminal; retry moves it back to `PENDING`.

"In-flight" states (a worker holds the task): `ASSIGNED`, `RUNNING`, `TESTING`, `REVIEWING`. Whenever a task leaves these, `worker_id` is set back to `NULL`.

---

## 3. Tables

Legend for "Written by": **O** = owner via CLI/API, **W** = worker via API, **S** = system (the reaper inside the control plane).

### `projects`

A git repository the platform works on.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | Auto-generated |
| `slug` | text UNIQUE | Lowercase `a-z0-9-`. Used in folder names: `knowledge/projects/<slug>`, `workspaces/<slug>` |
| `name` | text | Display name |
| `repo_url` | text | Anything `git clone` accepts (URL or local path) |
| `default_branch` | text | Default `main`. Treated as the **protected** branch |
| `stack` | text | e.g. `go`. Selects `knowledge/stacks/<stack>`, and is the default required capability for tasks |
| `autonomy_level` | autonomy_level | Default `conservative` |
| `test_command` | text | Run in the worktree after the agent finishes, and again after the merge. Empty = skip |
| `created_at` | timestamptz | |

Written by: **O** (`empire project create`). There's no update or delete endpoint yet.

### `tasks`

One unit of work.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | Also used in the branch name `ai/task-<id>` |
| `project_id` | FK → projects | |
| `title` | text, not empty | Also the commit message |
| `description` | text | The request, sent to the agent |
| `status` | task_status | Default `PENDING`. See transitions above |
| `stage` | text | `implement` or `merge`: what the worker does on its next claim |
| `required_capabilities` | text[] | A worker can claim only if its capabilities include all of these. Defaults to `[project.stack]` |
| `context_docs` | text[] | Paths under `knowledge/projects/<slug>/` to give the agent |
| `feedback` | text | Your comment from the last "request changes". Added to the prompt |
| `worker_id` | FK → workers, nullable | Set only while in-flight |
| `attempts` | int | +1 on every claim. Reset by approve / request-changes / retry. At ≥3, a requeue fails the task |
| `last_error` | text | Why it failed |
| `created_at`, `updated_at` | timestamptz | |

Indexes: `tasks_claimable` (partial, `status = 'PENDING'`), `tasks_worker` (partial, `worker_id IS NOT NULL`).

| Column(s) | Written by |
|-----------|------------|
| new row | **O** `POST /tasks` |
| `status` | **O** cancel/retry/decide, **W** claim/transition/authorize, **S** reaper |
| `worker_id`, `attempts` | **W** claim (set), any move out of in-flight (cleared) |
| `stage` | **W** authorize (when parking), **O** request-changes (back to `implement`) |
| `feedback` | **O** request-changes |
| `last_error` | **W** transition to FAILED, **S** requeue when attempts are exhausted |

### `task_dependencies`

"Task A can't start until task B is COMPLETED."

| Column | Type | Notes |
|--------|------|-------|
| `task_id` | FK → tasks (cascade delete) | The waiting task |
| `depends_on_task_id` | FK → tasks | Must be in the **same project** (checked by the API) |

PK `(task_id, depends_on_task_id)`, and a task can't depend on itself. Written by: **O** (`empire task create -after ID`). The claim query skips tasks with any dependency not yet `COMPLETED`.

### `workers`

Execution processes.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | Sent by the worker as `X-Worker-ID` |
| `name` | text UNIQUE | `EMPIRE_WORKER_NAME` (default: hostname). Re-registering with the same name reuses the row |
| `capabilities` | text[] | `EMPIRE_WORKER_CAPS`, e.g. `{go}` |
| `status` | text | `online` / `offline` |
| `last_heartbeat_at` | timestamptz | Updated every 10s |
| `created_at` | timestamptz | |

Written by: **W** (register, heartbeat) and **S** (the reaper sets `offline` after `EMPIRE_STALE_AFTER`).

### `agent_runs`

One execution of the coding agent.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `task_id` | FK → tasks | |
| `worker_id` | FK → workers | |
| `model` | text | `claude-code`, `claude-code:<model>`, or `fake` |
| `context_files` | text[] | Exactly which knowledge files the agent saw |
| `started_at` | timestamptz | |
| `finished_at` | timestamptz, nullable | `NULL` = still running, or the worker crashed mid-run |
| `exit_status` | int, nullable | The agent process's exit code (-1 if it never started) |
| `log_path` | text | Full agent output on the **worker's** disk: `workspaces/logs/task-N-run-M.log` |
| `tokens` | bigint | Input + output + cache tokens (as reported by Claude) |
| `cost_usd` | numeric(12,6) | As reported by Claude |

Written by: **W** (`POST /tasks/{id}/runs`, then `POST /runs/{id}/finish`).

### `approval_requests`

A gate waiting for (or already given) a human decision.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | The number you pass to `empire approve` |
| `project_id` | FK → projects | |
| `subject_type` | text | `task` in V1 |
| `subject_ref` | text | The task id, as text |
| `subject_version` | text | For merges: the **exact commit sha** you're approving. The worker merges only this sha |
| `gate` | text | The policy action, e.g. `merge_protected` |
| `status` | approval_status | Default `PENDING_APPROVAL` |
| `summary` | text | What you see in `empire approvals` (includes the diffstat) |
| `requested_by` | text | e.g. `worker:1`. This actor can never decide the request |
| `created_at` | timestamptz | |

Unique partial index `approval_requests_one_open`: at most **one** `PENDING_APPROVAL` row per (subject_type, subject_ref, gate).

Written by: **W** (insert via authorize), **O** (status via decide; set to `REJECTED` when the task is cancelled).

### `approval_decisions`

The human answer. In practice a request has at most one decision row, because the API only accepts decisions on `PENDING_APPROVAL` requests (no DB constraint enforces it).

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `approval_request_id` | FK → approval_requests | |
| `decision` | approval_status | Never `PENDING_APPROVAL` (check constraint) |
| `comment` | text | Required for `CHANGES_REQUESTED` |
| `decided_by` | text | `owner` in V1 |
| `decided_at` | timestamptz | |

Written by: **O** only (`empire approve | request-changes | reject`).

### `audit_log`

The append-only history of every state change.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `actor_type` | text | `owner`, `worker`, or `system` |
| `actor_id` | text | `owner`, the worker id (e.g. `1`), or `system` |
| `action` | text | See the table below |
| `target` | text | `project:N`, `task:N`, `worker:N`, `approval:N` |
| `payload` | jsonb | Action-specific details |
| `created_at` | timestamptz | |

Triggers `audit_log_no_update_delete` and `audit_log_no_truncate` raise an error on any `UPDATE`, `DELETE`, or `TRUNCATE`. Every audit row is written **in the same transaction** as the change it describes, so a change without an audit row can't exist.

| `action` | `target` | Written when | Payload |
|----------|----------|--------------|---------|
| `project.create` | project | Project created | full project |
| `task.create` | task | Task created | full task, `depends_on` |
| `task.status` | task | **Any** status change | `from`, `to`, plus `stage` / `error` / `reason` / `approval_id` |
| `worker.register` | worker | Worker starts | full worker |
| `worker.offline` | worker | Reaper found it silent | none |
| `agent_run.start` | task | Agent starting | `run_id`, `model`, `context_files` |
| `agent_run.finish` | task | Agent ended | `run_id`, `result` (exit, tokens, cost, log) |
| `policy.allow` | task | Action allowed | `action`, plus `autonomy` or `approval_id` |
| `approval.request` | approval | Gate opened | full request |
| `approval.decide` | approval | You decided | `decision`, `comment` |

---

## 4. Useful queries

```sql
-- What is running right now?
SELECT t.id, p.slug, t.status, t.stage, w.name AS worker, t.title
FROM tasks t JOIN projects p ON p.id = t.project_id LEFT JOIN workers w ON w.id = t.worker_id
WHERE t.status IN ('ASSIGNED','RUNNING','TESTING','REVIEWING');

-- What needs me?
SELECT id, gate, subject_ref AS task, summary FROM approval_requests WHERE status = 'PENDING_APPROVAL';

-- Cost per project
SELECT p.slug, count(r.*) AS runs, sum(r.tokens) AS tokens, sum(r.cost_usd) AS usd
FROM agent_runs r JOIN tasks t ON t.id = r.task_id JOIN projects p ON p.id = t.project_id
GROUP BY p.slug;

-- Full history of one task
SELECT created_at, actor_type, actor_id, action, payload FROM audit_log
WHERE target = 'task:1' ORDER BY id;

-- Everything a given approval covered and who decided it
SELECT r.id, r.gate, r.subject_version, d.decision, d.comment, d.decided_by, d.decided_at
FROM approval_requests r LEFT JOIN approval_decisions d ON d.approval_request_id = r.id
WHERE r.subject_type = 'task' AND r.subject_ref = '1';

-- Failed tasks and why
SELECT id, title, attempts, last_error FROM tasks WHERE status = 'FAILED';
```

## 5. Changing the schema

1. Add `migrations/000003_<name>.up.sql` and a matching `.down.sql`.
2. Run `make migrate` (or `make migrate-down` to roll back one step).
3. If you added a column to a table, add the field to the struct in [internal/api/types.go](../internal/api/types.go). Queries use `SELECT *` and map columns by their `db:"…"` tag, so a column without a matching field causes a loud error.
4. `make test` recreates the `empire_test` database from all migrations, so it checks the new migration too.
