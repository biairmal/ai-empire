# Database Model

PostgreSQL 17 is the **single source of truth** for platform state (spec §31). Only the control plane connects to it. Workers and the CLI go through the HTTP API.

- Schema source: [migrations/](../migrations/)
  - `000002_core`: tasks, workers, approvals, audit
  - `000003_clients_repositories`: clients and repositories
  - `000004_agent_run_summary`: `agent_runs.summary`
  - `000005_workflows_documents`: requests, task kinds, document approvals, knowledge graph index
  - `000006_task_changed_files`: `tasks.changed_files`
  - `000007_v2_interface`: per-worker tokens and project limits, `agent_runs.log_tail`, the `notifications` outbox
- Connection (dev): `postgres://empire:empire@localhost:5433/empire`
- Open a SQL shell: `docker compose exec postgres psql -U empire -d empire`

Source code, documents and knowledge are **not** in the database. They live in git.

---

## 1. Entity relationship diagram

```text
┌────────────────────┐
│      clients       │   (optional) the confidentiality boundary
│────────────────────│
│ id  PK, slug UNIQUE│
│ default_autonomy   │
└─────────┬──────────┘
          │ 0..1
          │
          │ *
┌─────────▼──────────┐ 1          * ┌──────────────────────────┐
│      projects      │─────────────►│       repositories       │
│────────────────────│              │──────────────────────────│
│ id  PK, slug UNIQUE│              │ id  PK                   │
│ client_id FK (null)│              │ project_id FK            │
│ autonomy_level     │              │ name (unique in project) │
└──┬──────────────┬──┘              │ repo_url, default_branch │
   │ 1            │ 1               │ stack, test_command      │
   │              │                 └────────────┬─────────────┘
   │ *            │ *                            │ 1
   │   ┌──────────▼───────────────┐              │
   │   │    approval_requests     │              │ *
   │   │──────────────────────────│  ┌───────────▼──────────────────┐
   │   │ subject_type = 'task'    │  │            tasks             │
   │   │ subject_ref  = task id ─ ┼ ►│──────────────────────────────│
   │   │ gate, status             │  │ id  PK                       │
   │   │ subject_version (sha)    │  │ project_id FK ─┐ composite FK│
   │   └──────────┬───────────────┘  │ repository_id ─┘ (same proj.)│
   │              │ 1                │ status, stage                │
   │              │ *                │ worker_id FK (null)          │
   │   ┌──────────▼───────────────┐  │ attempts, feedback           │
   │   │   approval_decisions     │  └──┬────────▲────────┬─────────┘
   │   │ decision, comment        │     │ 1      │ 1      │ 1
   │   │ decided_by               │     │ *      │ *      │ *
   │   └──────────────────────────┘  ┌──┴────────┴──┐  ┌──▼───────────────────┐
   └──────────────────────────────►  │task_dependen-│  │      agent_runs      │
          (projects 1 ── * tasks)    │cies          │  │ task_id, worker_id   │
                                     │ task_id      │  │ model, context_files │
┌─────────────────────┐              │ depends_on   │  │ tokens, cost_usd     │
│       workers       │ 1        *   └──────────────┘  └──────────▲───────────┘
│ id PK, name UNIQUE  │─────────────────────── (summary) ──────────┘
│ capabilities[]      │  (also tasks.worker_id while in flight)
│ status, heartbeat   │
└─────────────────────┘

┌──────────────────────────┐
│        audit_log         │  append-only (trigger). No FKs: points at any row
│ actor, action, target    │  as text, e.g. "task:1", "project:2", "client:3"
│ payload jsonb            │
└──────────────────────────┘
```

**The hierarchy:** client (optional) → projects → repositories → tasks. A task always belongs to **one project**; a code task also belongs to **one repository of that project**. The database enforces this with a check constraint (`kind = code` ⇔ repository set) and a composite foreign key `(repository_id, project_id) → repositories (id, project_id)`.

The V3 tables (requests, document_approvals, documents, doc_edges) are described in section 3; they hang off projects and tasks as shown there.

**Why approvals point at their subject by text:** an approval request is either a merge gate (`subject_type = task`) or a document approval (`subject_type = document`, subject = document key). Document approvals also carry `task_id` when a workflow task produced the documents.

Also present: `schema_migrations`, which golang-migrate manages. It records the current schema version (6).

---

## 2. Enum types

| Type | Values |
|------|--------|
| `task_status` | `PENDING`, `ASSIGNED`, `RUNNING`, `TESTING`, `REVIEWING`, `WAITING_FOR_HUMAN`, `COMPLETED`, `FAILED`, `CANCELLED` |
| `approval_status` | `PENDING_APPROVAL`, `APPROVED`, `CHANGES_REQUESTED`, `REJECTED` |
| `autonomy_level` | `high`, `medium`, `conservative` |

`REVIEWING` is used while the AI reviewer checks a code change.

### Task status transitions

These are enforced in [internal/task/state.go](../internal/task/state.go). Every change goes through one function, `move()`, in [internal/controlplane/tasks.go](../internal/controlplane/tasks.go).

```text
                      ┌───────────── requeue (crash / restart / approve / changes) ──────┐
                      │                                                                  │
 PENDING ──► ASSIGNED ──► RUNNING ──► TESTING ──► REVIEWING ──► WAITING_FOR_HUMAN ──► PENDING ───┘
    │           │           │  ▲         │                            │  │
    │           │           │  └─────────┤                            │  └──► CANCELLED
    │           │           │            │                            └──► COMPLETED (document approved)
    │           ▼           ▼            ▼
    │        FAILED ◄───────┴────────────┤          COMPLETED  (worker: code tasks only, from RUNNING/TESTING/
    │           │                        │                      REVIEWING, and only in stage=merge)
    │           └──► PENDING (retry)     │
    ▼                                    ▼
 CANCELLED ◄──── (owner cancel from any non-final state except FAILED)
```

Terminal states: `COMPLETED` and `CANCELLED`. `FAILED` isn't terminal; retry moves it back to `PENDING`.

"In-flight" states (a worker holds the task): `ASSIGNED`, `RUNNING`, `TESTING`, `REVIEWING`. Whenever a task leaves these, `worker_id` is set back to `NULL`.

---

## 3. Tables

Legend for "Written by": **O** = owner via CLI/API (some actions also by Hermes, audited as `hermes`), **W** = worker via API, **S** = system (the reaper and the notifier inside the control plane).

**Slug rule** (clients and projects): lowercase `a-z0-9-`, starting with a letter or digit, and **containing at least one letter**. That lets every API path take either the slug or the numeric id (`/projects/guest` or `/projects/3`).

### `clients`

Who the work is for (spec §11B). Optional: personal projects have no client.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `slug` | text UNIQUE | Also the knowledge folder: `knowledge/clients/<slug>/` |
| `name` | text | |
| `default_autonomy_level` | autonomy_level | Default `conservative`. New projects of this client start with it |
| `created_at` | timestamptz | |

Written by: **O** (`empire client create`).

### `projects`

A product (spec §11A). Its code lives in one or more repositories.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `slug` | text UNIQUE | Used in `knowledge/projects/<slug>` and `workspaces/<slug>/…` |
| `name` | text | Display name |
| `client_id` | FK → clients, nullable | `NULL` = personal/internal project |
| `autonomy_level` | autonomy_level | Given at creation, or else the client's default, or else `conservative`. Moving the project to another client does **not** change it |
| `created_at` | timestamptz | |

Written by: **O** (`empire project create`; `empire project set` changes `name` / `autonomy_level`; `empire project move` changes `client_id`).

A move is refused when the project has task dependencies with a project that would then belong to a different client.

### `repositories`

A git repository of a project.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `project_id` | FK → projects | |
| `name` | text | `a-z0-9-`, unique within the project (e.g. `backend`, `frontend`) |
| `repo_url` | text | Anything `git clone` accepts. **Must be pushable:** a remote or a bare repo, not a normal local repo with the branch checked out |
| `default_branch` | text | Default `main`. Treated as the **protected** branch |
| `stack` | text | e.g. `go`, `node`. Selects `knowledge/stacks/<stack>`, and is the default required capability for tasks |
| `test_command` | text | Run in the worktree after the agent finishes, and again after the merge. Empty = skip |
| `created_at` | timestamptz | |

Constraints: `UNIQUE (project_id, name)`, plus `UNIQUE (id, project_id)`, which is the target of the tasks composite FK.

Written by: **O** (`empire repo add`; `empire repo set` changes `repo_url`, `default_branch`, `stack`, `test_command`). There's no rename or delete yet. Running tasks keep the settings they were claimed with.

Migration 000003 converted each pre-existing project into a project with one repository named after the project slug.

### `tasks`

One unit of work. **Code** tasks run in exactly one repository; **document**, **plan** and **revise** tasks produce documents and have no repository.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | Also used in the branch name `ai/task-<id>` |
| `project_id` | FK → projects | |
| `repository_id` | bigint, nullable | Set exactly for `kind = code` (check constraint), and then a repository **of `project_id`** (composite FK) |
| `kind` | text | `code`, `document`, `plan`, or `revise` |
| `role` | text | Agent role: `developer` (default), `product-manager`, `architect`, `planner` |
| `request_id` | FK → requests, nullable | The request (workflow run) this task belongs to; `NULL` for stand-alone tasks |
| `step` | text | The workflow step that created it |
| `doc_type` | text | Document tasks: the document type to write |
| `revises` | text | Revise tasks: the key of the approved document to version (`projects/x/PRD-001`) |
| `output_docs` | text[] | Keys of the documents this task wrote |
| `review` | boolean | Code tasks: run the AI reviewer before the merge gate |
| `review_rounds` | int | How often the AI reviewer sent the task back (max 2) |
| `merge_sha` | text | Code tasks: the commit on the default branch after merging (traceability) |
| `changed_files` | text[] | Code tasks: files the merge changed (impact analysis) |
| `title` | text, not empty | Also the commit message |
| `description` | text | The request, sent to the agent |
| `status` | task_status | Default `PENDING`. See transitions above |
| `stage` | text | `implement` or `merge`: what the worker does on its next claim |
| `required_capabilities` | text[] | A worker can claim only if its capabilities include all of these. Defaults to `[repository.stack]` |
| `context_docs` | text[] | Paths under `knowledge/projects/<project slug>/` to give the agent |
| `feedback` | text | Your comment from the last "request changes". Added to the prompt |
| `worker_id` | FK → workers, nullable | Set only while in-flight |
| `attempts` | int | +1 on every claim. Reset by approve / request-changes / retry. At ≥3, a requeue fails the task |
| `last_error` | text | Why it failed |
| `created_at`, `updated_at` | timestamptz | |

Indexes: `tasks_claimable` (partial, `status = 'PENDING'`), `tasks_worker` (partial, `worker_id IS NOT NULL`).

| Column(s) | Written by |
|-----------|------------|
| new row | **O** `POST /tasks` (repository: by name, optional if the project has exactly one), or the workflow engine (request steps, approved plans) |
| `status` | **O** cancel/retry/decide, **W** claim/transition/authorize, **S** reaper |
| `worker_id`, `attempts` | **W** claim (set), any move out of in-flight (cleared) |
| `stage` | **W** authorize (when parking), **O** request-changes (back to `implement`) |
| `feedback` | **O** request-changes |
| `last_error` | **W** transition to FAILED, **S** requeue when attempts are exhausted |
| `output_docs` | **W** accepted document upload |
| `review_rounds`, `feedback` | **W** AI review asking for changes |
| `merge_sha`, `changed_files` | **W** transition to COMPLETED after the merge |

### `task_dependencies`

"Task A can't start until task B is COMPLETED."

| Column | Type | Notes |
|--------|------|-------|
| `task_id` | FK → tasks (cascade delete) | The waiting task |
| `depends_on_task_id` | FK → tasks | Any project, but **the same client** (or both client-less). Checked by the API |

PK `(task_id, depends_on_task_id)`, and a task can't depend on itself. Written by: **O** (`empire task create -after ID`). The claim query skips tasks with any dependency not yet `COMPLETED`.

Dependencies only order work. They never share knowledge between projects (spec §11A).

### `workers`

Execution processes.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | Sent by the worker as `X-Worker-ID` (ignored for workers that have their own token) |
| `name` | text UNIQUE | `EMPIRE_WORKER_NAME` (default: hostname). Re-registering with the same name reuses the row |
| `capabilities` | text[] | `EMPIRE_WORKER_CAPS`, e.g. `{go,node}` |
| `status` | text | `online` / `offline` |
| `last_heartbeat_at` | timestamptz | Updated every 10s |
| `token_hash` | text UNIQUE, nullable | sha256 of the worker's own token (`empire worker add`). `NULL` = uses the shared token |
| `project_ids` | bigint[] | Projects it may claim tasks from. Empty = any |
| `created_at` | timestamptz | |

Written by: **O** (`POST /workers`: token and projects), **W** (register, heartbeat) and **S** (the reaper sets `offline` after `EMPIRE_STALE_AFTER`).

### `agent_runs`

One execution of an agent (developer, reviewer, product manager, architect, planner).

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
| `summary` | text | The agent's own account of what it did (Claude's final message, up to 10 000 bytes). Also shown in the merge approval request |
| `role` | text | Which role ran: a code task has `developer` runs and, with AI review, `reviewer` runs |
| `log_tail` | text | Last 8 KB of the agent log, so it can be read without access to the worker's disk |

Written by: **W** (`POST /tasks/{id}/runs`, then `POST /runs/{id}/finish`).

### `approval_requests`

A gate waiting for (or already given) a human decision. There is **one merge gate per repository** (spec §11A).

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | The number you pass to `empire approve` |
| `project_id` | FK → projects, nullable | `NULL` for global documents (guidelines, roles) |
| `task_id` | FK → tasks, nullable | Document approvals: the task that wrote the documents |
| `subject_type` | text | `task` (merge gates) or `document` |
| `subject_ref` | text | The task id, or the primary document key (`projects/guest/TD-001`) |
| `subject_version` | text | Merges: the **exact commit sha** you're approving. Documents: `v<version>@<content hash>` |
| `covers` | text[] | Documents approved together (e.g. a design and its ADRs): `<key>@v<version>@<hash>` each |
| `gate` | text | The policy action (`merge_protected`) or `document_approval` |
| `status` | approval_status | Default `PENDING_APPROVAL` |
| `summary` | text | What you see in `empire approvals`: `Merge ai/task-7 (abc123) into backend:main …`, then the agent summary, then the diffstat |
| `requested_by` | text | `worker:N` or `owner`. An AI actor can never decide; the owner may decide what they submitted |
| `created_at` | timestamptz | |

Unique partial index `approval_requests_one_open`: at most **one** `PENDING_APPROVAL` row per (subject_type, subject_ref, gate).

Written by: **W** (insert via authorize, or an accepted document upload), **O** (`empire docs submit`; status via decide; set to `REJECTED` when the task is cancelled).

### `approval_decisions`

The human answer. In practice a request has at most one decision row, because the API only accepts decisions on `PENDING_APPROVAL` requests (no DB constraint enforces it).

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `approval_request_id` | FK → approval_requests | |
| `decision` | approval_status | Never `PENDING_APPROVAL` (check constraint) |
| `comment` | text | Required for `CHANGES_REQUESTED` |
| `decided_by` | text | `owner` |
| `decided_at` | timestamptz | |

Written by: **O** only (`empire approve | request-changes | reject`).

### `requests`

A plain-language request driven through a workflow (spec §7).

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | `empire request get <id>` |
| `project_id` | FK → projects | |
| `repository_id` | bigint, nullable | Only for workflows that start with code (`quick-fix`); must belong to the project |
| `workflow` | text | `feature`, `quick-fix`, `change`, or your own (from `workflows/`) |
| `title`, `description` | text | Your words; given to every agent in the request |
| `status` | text | `active`, `completed`, or `cancelled` |
| `current_step` | text | The step the request is on |
| `created_at`, `updated_at` | timestamptz | |

Written by: **O** (`empire request create / cancel`) and the workflow engine (step and status as tasks finish).

### `document_approvals`

The authoritative record of every **approved document version** (spec §12). Append-only (trigger).

| Column | Type | Notes |
|--------|------|-------|
| `scope`, `doc_id` | text | Which document (`projects/guest`, `PRD-001`) |
| `version` | int | Approved version |
| `hash` | text | Content fingerprint (SHA-256 of the document without its `status` field and generated Relations block). Any later edit changes it |
| `approval_request_id` | FK → approval_requests | The decision that approved it |
| `approved_by`, `approved_at` | text, timestamptz | |

PK `(scope, doc_id, version)`. The validator compares every document against this table: an approved version whose fingerprint changed is an error, and a new version must come from an approved change request.

### `documents` and `doc_edges`

The **knowledge graph index** (spec §14): a cache rebuilt from the Markdown files by the control plane. It's rebuilt after every document change it makes, at start-up, by `empire docs reindex`, and by the post-commit hook. Useful for SQL queries; the platform itself reads the files.

| `documents` column | Notes |
|--------------------|-------|
| `scope`, `doc_id` (PK) | e.g. `projects/guest`, `TD-001` |
| `type`, `title`, `path`, `status`, `version`, `hash` | From the front matter; `path` is relative to `knowledge/` |

| `doc_edges` column | Notes |
|--------------------|-------|
| `from_scope`, `from_id` | The document that declares the relationship |
| `rel` | `satisfies`, `implements`, `depends_on`, `affects`, `derived_from`, `supersedes`, `contradicts`, `tested_by`, `documents`, `references` |
| `to_scope`, `to_id` | The resolved target |

```text
requests 1 ── * tasks ── output_docs ──► documents ◄── doc_edges ──► documents
                  │                          ▲
                  └── merge_sha, changed_files     document_approvals (approved versions)
```

### `notifications`

Outbox for owner notifications (V2). `audit()` adds a row in the same transaction as the audit entry it announces. The control plane sends due rows to ntfy.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `audit_id` | FK → audit_log | The event: `approval.request`, `task.status` to FAILED/COMPLETED, `worker.offline`, `request.completed` |
| `attempts` | int | Failed sends |
| `last_error` | text | Last send error, or `expired` |
| `next_attempt_at` | timestamptz | Backoff: now + 2^attempts s, at most 1 h |
| `sent_at` | timestamptz, nullable | `NULL` = still to send. Rows older than a day are expired |
| `created_at` | timestamptz | |

Written by: **S**.

### `audit_log`

The append-only history of every state change.

| Column | Type | Notes |
|--------|------|-------|
| `id` | bigint PK | |
| `actor_type` | text | `owner`, `worker`, or `system` |
| `actor_id` | text | `owner`, the worker id (e.g. `1`), or `system` |
| `action` | text | See the table below |
| `target` | text | `client:N`, `project:N`, `request:N`, `task:N`, `worker:N`, `approval:N`, `document:<key>` |
| `payload` | jsonb | Action-specific details |
| `created_at` | timestamptz | |

Triggers `audit_log_no_update_delete` and `audit_log_no_truncate` raise an error on any `UPDATE`, `DELETE`, or `TRUNCATE`. Every audit row is written **in the same transaction** as the change it describes.

| `action` | `target` | Written when | Payload |
|----------|----------|--------------|---------|
| `client.create` | client | Client created | full client |
| `project.create` | project | Project created | full project, `client` |
| `project.move_client` | project | Project moved to another client or none | `from_client_id`, `to_client_id`, `to_client` |
| `project.update` | project | Project name/autonomy changed | `before`, `after` |
| `repository.create` | project | Repository added | full repository |
| `repository.update` | project | Repository settings changed | `before`, `after` |
| `task.create` | task | Task created | full task, `repository`, `depends_on` |
| `task.status` | task | **Any** status change | `from`, `to`, plus `stage` / `error` / `reason` / `approval_id` |
| `worker.register` | worker | Worker starts | full worker |
| `worker.offline` | worker | Reaper found it silent | none |
| `agent_run.start` | task | Agent starting | `run_id`, `model`, `context_files` |
| `agent_run.finish` | task | Agent ended | `run_id`, `result` (exit, tokens, cost, log) |
| `policy.allow` | task | Action allowed | `action`, plus `autonomy` or `approval_id` |
| `approval.request` | approval | Gate opened | full request |
| `approval.decide` | approval | You decided | `decision`, `comment` |
| `request.create` | request | Request created | full request |
| `request.completed` / `request.cancelled` | request | All steps done / a step was rejected or you cancelled | `reason` |
| `task.output` | task | A document upload passed validation | `documents` |
| `task.review` | task | The AI reviewer reported | `verdict`, `round`, `summary` |
| `document.create` | document | `empire docs new` | `path`, `type` |
| `document.approved` / `document.changes_requested` / `document.rejected` | document | You decided on a document | `version`, `hash`, `approval_id`, `comment` |
| `document.withdrawn` | document | Its request or task was cancelled before approval | `task_id` |

---

## 4. Useful queries

```sql
-- The whole hierarchy
SELECT coalesce(c.slug, '(personal)') AS client, p.slug AS project, r.name AS repo, r.stack, r.repo_url
FROM projects p
LEFT JOIN clients c ON c.id = p.client_id
LEFT JOIN repositories r ON r.project_id = p.id
ORDER BY 1, 2, 3;

-- What is running right now?
SELECT t.id, p.slug || '/' || r.name AS repo, t.status, t.stage, w.name AS worker, t.title
FROM tasks t
JOIN projects p ON p.id = t.project_id
JOIN repositories r ON r.id = t.repository_id
LEFT JOIN workers w ON w.id = t.worker_id
WHERE t.status IN ('ASSIGNED','RUNNING','TESTING','REVIEWING');

-- What needs me?
SELECT id, gate, subject_ref AS task, summary FROM approval_requests WHERE status = 'PENDING_APPROVAL';

-- What is blocked, and by what?
SELECT d.task_id, t.title, d.depends_on_task_id, dt.status AS dependency_status
FROM task_dependencies d
JOIN tasks t ON t.id = d.task_id
JOIN tasks dt ON dt.id = d.depends_on_task_id
WHERE t.status = 'PENDING' AND dt.status <> 'COMPLETED';

-- Cost per client and project
SELECT coalesce(c.slug, '(personal)') AS client, p.slug AS project,
       count(r.*) AS runs, sum(r.tokens) AS tokens, sum(r.cost_usd) AS usd
FROM agent_runs r
JOIN tasks t ON t.id = r.task_id
JOIN projects p ON p.id = t.project_id
LEFT JOIN clients c ON c.id = p.client_id
GROUP BY 1, 2 ORDER BY 1, 2;

-- Full history of one task
SELECT created_at, actor_type, actor_id, action, payload FROM audit_log
WHERE target = 'task:1' ORDER BY id;

-- Everything a given approval covered and who decided it
SELECT r.id, r.gate, r.subject_version, d.decision, d.comment, d.decided_by, d.decided_at
FROM approval_requests r LEFT JOIN approval_decisions d ON d.approval_request_id = r.id
WHERE r.subject_type = 'task' AND r.subject_ref = '1';

-- Requests and where they are
SELECT r.id, p.slug, r.workflow, r.status, r.current_step, r.title
FROM requests r JOIN projects p ON p.id = r.project_id ORDER BY r.id DESC;

-- Approved document versions, newest first
SELECT scope, doc_id, version, approved_by, approved_at FROM document_approvals ORDER BY approved_at DESC;

-- What links to PRD-001 in project guest?
SELECT from_id, rel FROM doc_edges WHERE to_scope = 'projects/guest' AND to_id = 'PRD-001';

-- Which merged code touched a file?
SELECT id, title, merge_sha FROM tasks WHERE 'internal/app/router.go' = ANY(changed_files);

-- Agent cost per role
SELECT role, count(*) AS runs, sum(cost_usd) AS usd FROM agent_runs GROUP BY role ORDER BY usd DESC;

-- Failed tasks and why
SELECT id, title, attempts, last_error FROM tasks WHERE status = 'FAILED';
```

## 5. Changing the schema

1. Add `migrations/000007_<name>.up.sql` and a matching `.down.sql`.
2. Run `make migrate` (or `make migrate-down` to roll back one step).
3. If you added a column to a table, add the field to the struct in [internal/api/types.go](../internal/api/types.go). Queries use `SELECT *` and map columns by their `db:"…"` tag, so a column without a matching field causes a loud error.
4. `make test` recreates the `empire_test` database from all migrations, so it checks the new migration too.

Note: rolling back `000003` is lossy for projects with several repositories. Each project keeps only its first repository's settings.
