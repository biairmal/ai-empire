# AI Empire: Start Here

This page explains what exists **right now** (V1 core platform + V3 AI software organization) and how the parts fit together. Read it first, then go deeper:

- [documents.md](documents.md): **which document to use when**, how to write, approve, change, and hand them to clients
- [workflows.md](workflows.md): how a plain-language request becomes documents, a plan, and reviewed code
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
│  - runs the workflows    │
│  - validates, stores and ├──── reads / writes ── knowledge/  (Markdown + Git)
│    indexes documents     │
│  - builds agent context  │
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
│    in its role (PM,      │
│    architect, planner,   │
│    developer)            │
│  - runs the AI reviewer  │──► claude -p   (read-only)
│  - runs tests, commits,  │
│    pushes, merges        │
│  - document tasks run in │
│    a scratch folder      │
└──────────────────────────┘
```

**The three programs:**

| Program | Who uses it | What it does |
|---------|-------------|--------------|
| `controlplane` | Everyone talks to it | The "AI Dev OS". Stores all state, runs the workflow engine, enforces the task state machine and approval rules, validates and versions documents, keeps the knowledge graph, writes the audit log. |
| `worker` | Runs in the background | Picks up tasks. Code tasks: Claude Code in an isolated git worktree, tests, AI review, push, merge after approval. Document tasks: Claude writes the document in a scratch folder and uploads it for validation. |
| `empire` | You | A command-line remote control for the control plane. |

**The supporting pieces:**

| Piece | What it is |
|-------|------------|
| PostgreSQL | Runs in Docker (`make up`). Holds clients, projects, repositories, requests, tasks, workers, approvals, approved document versions, the knowledge graph index, runs, and the audit log. |
| `migrate` | A one-shot Docker container (`make migrate`) that applies `migrations/*.sql`. |
| `knowledge/` | Markdown files: global rules, agent roles, stack and client knowledge, **project documents** (PRDs, designs, …), plus the document templates and contracts. |
| `workflows/` | Workflow definitions (YAML): `feature`, `quick-fix`, `change`. |
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
| **Request** | Something you want, in plain language ("QR ticket validation"). It runs through a **workflow**. |
| **Workflow / step** | An ordered recipe (`feature`, `quick-fix`, `change`). Each step creates tasks: a document, a plan, revisions, or code. |
| **Task kind** | `code` (in a repository), `document`, `plan` (becomes code tasks), or `revise` (new version of an approved document). |
| **Role** | Which agent does a task: product-manager, architect, planner, developer, reviewer. Defined in `knowledge/roles/`. |
| **Document / contract / template** | A Markdown file with a YAML header (PRD, design, ADR, …). Its **contract** says what it must contain; its **template** is the starting point. |
| **Approved version** | A document version you approved. The platform stores its fingerprint; changing it requires a **change request**. |
| **Knowledge graph** | The typed links between documents (`satisfies`, `depends_on`, …), plus tasks and commits. Used for context, trace, and impact analysis. |
| **AI review** | A separate, read-only reviewer agent that checks each code change before your merge gate. |

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
knowledge/roles/<task role>.md         the agent's role definition
knowledge/stacks/<repository stack>/   frontend task → stacks/node, never stacks/go (document tasks: all project stacks)
knowledge/clients/<project's client>/  only for that client's projects
knowledge/projects/<project>/<doc>     the task's documents, plus what they link to
                                       (the PRD a design satisfies, its ADRs, API/DB specs, test plans)
```

Repository-specific knowledge lives in the repository itself (README, CLAUDE.md, docs/); the agent reads it in its worktree.

A feature that spans repositories is one task per repository, chained with `-after`. You approve each repository's merge separately, and a task only starts once the tasks it depends on are merged. A chain may cross projects, but never clients.

---

## 3. The life of a request

The usual way to get work done is a **request**. The `feature` workflow runs like this; details are in [workflows.md](workflows.md).

```text
empire request create -project guest "QR ticket validation"
   │
   ├─ prd        document task → Product Manager agent writes PRD-001
   │             → validated (bad output goes back to the agent) → approval ──► YOU
   ├─ design     Architect agent writes TD-001 (+ ADRs), linked to PRD-001 → approval ──► YOU
   ├─ plan       Planner agent writes PLAN-001 with a work breakdown → approval ──► YOU
   │             → one code task per work item, in dependency order
   └─ implement  per task: developer agent → tests → AI reviewer (may send it back)
                 → merge gate ──► YOU → merged (commit + changed files recorded)
```

Afterwards:

- `empire trace commit:<sha>` explains why the code exists: task → plan → design → PRD → your request.
- `empire impact PRD-001` shows what a change to the PRD would touch.
- `empire docs export` produces client-ready copies of the approved documents.

Each code task inside a request works exactly like a stand-alone task (next section).

## 4. The life of one code task

This is what happened in the real V1 demo run, step by step, with the database rows each step touches.

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

## 5. Who is allowed to do what

| | You (owner token) | Worker (worker token) | Agent (Claude) |
|---|---|---|---|
| Create clients / projects / repositories / tasks | ✅ | ❌ | ❌ |
| Move a project to another client | ✅ | ❌ | ❌ |
| Cancel / retry tasks | ✅ | ❌ | ❌ |
| Create requests, write and submit documents | ✅ | ❌ | ❌ |
| Approve / reject gates (including documents you wrote) | ✅ | ❌ | ❌ |
| Upload documents for its task (validated before storing) | ❌ | ✅ | ❌ |
| Write files in `knowledge/` | ✅ (by hand) | ❌ (only through the control plane) | ❌ (scratch folder only) |
| Claim tasks, report status | ❌ | ✅ (only tasks it holds) | ❌ |
| Mark a task `COMPLETED` | ❌ | Code tasks, only in the `merge` stage (documents complete through your approval) | ❌ |
| Edit files in the worktree | n/a | n/a | ✅ |
| Run shell / git commands | n/a | ✅ | ❌ (`acceptEdits` mode; the reviewer is read-only) |
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

> **Note:** the worker currently asks about only two actions: `push_branch` and `merge_protected`. Medium-risk actions exist in policy, but nothing detects them yet (for example, noticing that the agent added a dependency).

---

## 6. Everyday commands

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

empire workflows                    # available workflows
empire request create -project guest "QR ticket validation"
empire request get 1                # steps, tasks, documents, what waits for you
empire docs list -project guest     # documents and whether they are approved
empire docs validate -project guest # check documents (add -local to skip the control plane)
empire docs new -project guest -type test-plan -title "…" -owner "…"
empire docs submit TP-001 -project guest
empire trace PRD-001 -project guest # why it exists, what depends on it
empire impact PRD-001 -project guest
empire docs export -project guest   # client-ready copies in exports/guest

make test            # all tests (end-to-end tests use a separate empire_test DB)
make docs-check      # offline document validation
make hooks           # git hooks: validate knowledge on commit, reindex after
make down            # stop Postgres (data is kept in a Docker volume)
```

Reset everything: `make down && docker volume rm aiempire_pgdata && rm -rf workspaces`, then `make up && make migrate`.

---

## 7. Repository map

```text
AI Empire/
├── AI_SOFTWARE_DEV_EMPIRE.md   vision / requirements
├── ROADMAP.md                  plan with checkboxes
├── README.md                   quickstart
├── docs/                       ← you are here (README, documents, workflows, database, apps)
├── Makefile                    up, migrate, test, build, run-*, docs-check, hooks
├── docker-compose.yml          postgres + migrate
├── .env / .env.example         config and dev tokens
├── env.ps1                     Windows: `. .\env.ps1` loads .env + adds bin\ to PATH
├── .github/workflows/ci.yml    CI: vet, tests, offline document validation
├── scripts/hooks/              git hooks (pre-commit validate, post-commit reindex)
├── migrations/                 database schema (SQL)
├── workflows/                  feature.yaml, feature-ui.yaml, quick-fix.yaml, change.yaml, e2e-tests.yaml
├── cmd/
│   ├── controlplane/           main() for the control plane
│   ├── worker/                 main() for the worker
│   └── empire/                 the CLI
├── internal/
│   ├── api/                    JSON shapes shared by all three programs
│   ├── client/                 HTTP client (used by worker + CLI)
│   ├── controlplane/           HTTP handlers + all SQL, workflow engine, documents, graph
│   ├── worker/                 claim loop, git, agents, prompts, document tasks, e2e tests
│   ├── docs/                   document parsing, contracts, validator, versioning, graph, export
│   ├── workflow/               workflow definitions
│   ├── policy/                 risk levels, autonomy presets, "who may approve"
│   ├── task/                   task state machine
│   └── knowledge/              context bundle builder
├── knowledge/                  Markdown knowledge (open in Obsidian)
│   ├── global/  roles/         rules for every task; agent role definitions
│   ├── stacks/  clients/       stack and client knowledge
│   ├── templates/  contracts/  document templates and their rules
│   └── projects/<slug>/        each project's documents, by folder (requirements/, architecture/, …)
├── workspaces/                 worker scratch (git-ignored)
├── exports/                    `empire docs export` output (default location, git-ignored)
└── bin/                        built binaries (git-ignored)
```

## 8. Known limitations

- All workers share one token, and a worker's identity (`X-Worker-ID`) is self-declared.
- A worker runs one task at a time, and there's no per-project lock for running tasks in parallel on one machine.
- Cancelling a task closes its open gates as `REJECTED` without writing an `approval_decisions` row.
- Clients, projects and repositories can be created and edited (projects can be moved), but slugs/names cannot be renamed and nothing can be deleted yet.
- A feature spanning repositories gets one merge gate per repository; there is no combined all-or-nothing approval.
- Knowledge files edited by hand are re-indexed at control-plane start, by `empire docs reindex`, or by the post-commit hook, not instantly.
- Approved document versions are identified by a content fingerprint, not a git commit; commit `knowledge/` yourself to keep the history.
- Deployment is not automated: the Deployment Plan is a document, and `deploy_production` exists only as a policy action.
- No Tester, Documentation, or DevOps agent roles yet; no web UI, notifications, or Hermes (V2, deferred).
