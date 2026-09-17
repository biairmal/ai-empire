# AI Software Development Empire — Implementation Roadmap

Derived from [AI_SOFTWARE_DEV_EMPIRE.md](AI_SOFTWARE_DEV_EMPIRE.md). `§N` points to a section of that spec.

**How to use this file**
- Work top to bottom. Each milestone ends with a **Done when** check you can demo. Don't start the next milestone until that check passes.
- Tick `[x]` as you go. Commit this file along with the code.
- Build only what the current milestone needs (§40). Anything marked *(defer)* waits until a real need shows up.

---

## Phase 0 — Foundations

### M0.1 Repository & tooling
- [x] `git init` this workspace; add `.gitignore` (Go, `.env`, `workspaces/`)
- [x] Choose a layout: one Go module, a modular monolith (§6)
  ```text
  cmd/controlplane/    # HTTP API + scheduler
  cmd/worker/          # worker binary
  internal/            # project, task, workflow, approval, policy, audit, context, knowledge
  migrations/          # SQL migrations
  knowledge/           # Markdown knowledge repo (later its own git repo)
  ```
- [x] `docker-compose.yml` with PostgreSQL only (add Redis in M1.6 only if it's needed). Host port 5433
- [x] `Makefile` / `justfile` with `up`, `migrate`, `test`, `run-cp`, `run-worker`
- [x] Pick a migration tool: `golang-migrate`, run as a Compose service (`docker compose run --rm migrate`)

**Done when:** `make up && make migrate && make test` runs clean on an empty project.

### M0.2 Knowledge repository skeleton (§13, §21)
- [x] Create the scope folders:
  ```text
  knowledge/
  ├── global/            # engineering + security principles
  ├── stacks/go/         # stack knowledge
  ├── stacks/dotnet/
  ├── templates/         # document templates (§17)
  ├── contracts/         # document contracts (§16)
  └── projects/<slug>/   # requirements/ ux/ architecture/ decisions/ api/ database/ testing/ operations/
  ```
- [x] Write `global/engineering-principles.md` (copy §2) and `global/security-principles.md`
- [x] Define the frontmatter format for every document (§15): `type, id, project, status, version` plus relationship keys → [knowledge/contracts/frontmatter.md](knowledge/contracts/frontmatter.md)

**Done when:** you can open `knowledge/` in Obsidian and browse it. Nothing else is needed yet.

---

## V1 — Core Platform (§41)
**Goal:** human creates a task → one worker executes it autonomously under controlled authority.

### M1.1 Data model (§31)
- [x] `projects` (id, slug, name, repo_url, default_branch, stack, autonomy_level)
- [x] `tasks` (id, project_id, title, description, status, required_capabilities, created_at, updated_at)
- [x] `task_dependencies` (task_id, depends_on_task_id)
- [x] `workers` (id, name, capabilities, last_heartbeat_at, status)
- [x] `agent_runs` (id, task_id, worker_id, model, started_at, finished_at, exit_status, log_path, tokens, cost)
- [x] `approval_requests` (id, project_id, subject_type, subject_ref, subject_version, gate, status, requested_by, created_at)
- [x] `approval_decisions` (id, approval_request_id, decision, comment, decided_by, decided_at)
- [x] `audit_log` (id, actor_type, actor_id, action, target, payload jsonb, created_at). Append-only, enforced with a DB rule or trigger
- [x] Task status is a Postgres enum with the 9 states from §30
- [x] Approval status is an enum: `PENDING_APPROVAL, APPROVED, CHANGES_REQUESTED, REJECTED` (§8)

**Done when:** migrations apply, and a single SQL insert/select smoke test passes for each table.

### M1.2 Control Plane API: projects & tasks (§6, §33)
- [x] HTTP server (stdlib `net/http` is enough) with a static API token for auth (§34)
- [x] `POST/GET /projects`, `GET /projects/{id}`
- [x] `POST/GET /tasks`, `GET /tasks/{id}`, `POST /tasks/{id}/cancel`, `POST /tasks/{id}/retry`
- [x] A task state machine in one function that rejects illegal transitions; add a table-driven test for it
- [x] Every mutating endpoint writes an `audit_log` row (§9 auditable actions)
- [x] Minimal CLI (`empire task create ...`), see [README.md](README.md)

**Done when:** you can create a project and a task over HTTP, move it through legal states, and every action shows up in `audit_log`.

### M1.3 Approval gates & policy (§8, §10, §11)
- [x] `GET /approvals`, `GET /approvals/{id}`, `POST /approvals/{id}/approve|request-changes|reject`
- [x] Rule: the actor that requested an approval cannot decide it (§9, §26). Test it
- [x] Policy with three presets: `high`, `medium`, `conservative` (§11). Presets live in code ([internal/policy/policy.go](internal/policy/policy.go)); add per-project overrides when a project needs one
- [x] One function `policy.Requires(project, action) bool`. Workers call it; they don't hardcode rules
- [x] Classify the §10 actions (low/medium/high). High-risk actions always require approval, whatever the preset

**Done when:** a task that hits a gated action moves to `WAITING_FOR_HUMAN`, and continues only after `approve`.

### M1.4 Git integration & workspace isolation (§24, §29)
- [x] Worker clones/fetches the project repo into `workspaces/<project>/_base`
- [x] Each task gets its own `git worktree` on branch `ai/<task-id>`
- [x] Clean up the worktree after the task reaches a terminal state
- [x] The agent process runs with the worktree as its cwd; no other project paths get mounted or passed in
- [ ] *(defer)* Docker-per-task. Add it when a project needs toolchain isolation

**Done when:** two tasks on the same repo run side by side in separate worktrees without touching each other.

### M1.5 Context loading, V1 version (§22)
- [x] Resolver input: task → project → stack
- [x] Output: a single `.empire-context.md` written into the worktree (git-excluded), containing:
  `global/*` + `stacks/<project stack>/*` + `projects/<slug>/*` docs listed in task metadata
- [x] Hard rule: never include another project's folder or another stack's folder (§21). Test it
- [x] Log which files were included on the `agent_run`

**Done when:** a Go-stack task's context has no .NET or other-project content, and a test proves it.

### M1.6 Worker & scheduler (§27, §28, §30)
- [x] Worker binary: register → heartbeat loop → claim → prepare workspace → load context → run agent → validate → report → cleanup
- [x] Claiming: `SELECT ... FOR UPDATE SKIP LOCKED` in Postgres (no Redis needed for one worker)
- [x] Capability match: only claim tasks whose `required_capabilities ⊆ worker.capabilities`
- [x] Heartbeat every N seconds; the control plane marks workers stale after M missed beats and requeues their `RUNNING` tasks
- [x] Control-plane restart safety: all state lives in Postgres, and a restart just resumes
- [ ] *(defer)* Redis streams/queues. Add them when you have multiple workers and Postgres polling becomes a measured bottleneck

**Done when:** you kill the worker mid-task, restart it, and the task gets requeued and finishes.

### M1.7 One AI coding agent (§32)
- [x] Put a small interface behind the agent call: `Run(ctx, worktree, prompt) (Result, error)`
- [x] First implementation: Claude Code headless (`claude -p ...`) or another CLI agent, restricted to the worktree
- [x] Capture stdout/stderr to a log file, plus exit code, duration, and tokens/cost if available → `agent_runs`
- [x] Post-run validation: run the project's test command (from project config) → `TESTING` → pass/fail
- [x] On success: commit to `ai/<task-id>`, push the branch, set the task to `WAITING_FOR_HUMAN` with a **merge** approval request
- [x] On approve: merge (or open a PR) → `COMPLETED`. The agent never merges to a protected branch itself (§10)

**Done when (V1 goal):** `empire task create "add /health endpoint"` → the worker implements it, tests pass, a merge approval appears → you approve → the change is merged. Full trail in `audit_log`.

### M1.8 Clients & multi-repository projects (§11A, §11B)
Client → projects → repositories. A client (optional) is the confidentiality boundary. A project is a product (e.g. Guest Management), and its repositories are backend/frontend.

**Data model**
- [x] Migration `000003`: `clients` (id, slug UNIQUE, name, default_autonomy_level, created_at)
- [x] `projects.client_id` FK, nullable (NULL = personal/internal project)
- [x] `repositories` (id, project_id FK, name, repo_url, default_branch, stack, test_command, created_at), `UNIQUE (project_id, name)`
- [x] Backfill: one repository per existing project from its current repo columns, named after the project slug
- [x] `tasks.repository_id` FK (NOT NULL after backfill), and it must belong to the task's project
- [x] Drop `repo_url`, `default_branch`, `stack`, `test_command` from `projects` (they move to `repositories`). `autonomy_level` stays per project

**API & CLI**
- [x] `POST /clients`, `GET /clients[/{id}]` (owner), audited
- [x] `POST /projects` takes an optional `client` (slug). A new project inherits the client's default autonomy unless one is given
- [x] `POST /projects/{id}/client` to move a project to another client or to none (owner only, audited, explicit)
- [x] `POST /projects/{id}/repositories`, `GET /projects/{id}/repositories` (owner), audited
- [x] `POST /tasks` takes `repository` (name). Required when the project has more than one repo; defaults to the only one otherwise
- [x] Task `required_capabilities` defaults to the repository's stack
- [x] Claim returns `{task, project, repository}`
- [x] Merge approval summary names the repository: `Merge ai/task-N into backend:main`
- [x] CLI: `empire client create -slug -name [-autonomy]`, `empire client list`, `empire project create -slug -name [-client C] [-autonomy]`, `empire project move P -client C|-none`, `empire repo add -project P -name N -repo URL -stack S [-branch] [-test]`, `empire repo list -project P`, `empire task create -project P -repo N …`
- [x] Project list shows the client; task list shows the repository

**Worker & context**
- [x] Workspace layout: `workspaces/<project>/<repo>/_base` and `workspaces/<project>/<repo>/task-N`
- [x] The worker clones, tests, pushes, and merges using the repository's settings
- [x] Context resolver, in load order: `global/` → `stacks/<repository stack>/` → `clients/<project's client>/` (only if the project has a client) → the task's docs from `projects/<project slug>/`
- [x] Hard rule, with a test: never another client's folder, and no client folder at all for client-less projects
- [x] Knowledge folders stay flat: `knowledge/clients/<slug>/` next to `knowledge/projects/<slug>/`, so moving a project between clients never moves files
- [x] Add `knowledge/clients/README.md` and `knowledge/stacks/node/` (placeholder READMEs)
- [x] Update [knowledge/contracts/frontmatter.md](knowledge/contracts/frontmatter.md): add a `clients/<slug>/` → `client: <slug>` scope, and add client to the allowed-reference order (project → client → stack → global)
- [x] No repository knowledge scope: repo-specific knowledge stays in the repo (README, CLAUDE.md, docs/)

**Dependencies & gates (decided in §11A)**
- [x] Allow cross-project `depends_on`, as ordering only. Both projects must have the same client, or both have none; otherwise → 400
- [x] Cross-project dependencies never add the other project's knowledge to context. Test it
- [x] One merge gate per repository (already how it works). *(defer)* combined multi-repo approval until V3 workflows group tasks into features

**Tests & docs**
- [x] E2E: one project with two repos (different stacks). A task in each repo merges only into its own repo, and the frontend task's context has no Go stack files
- [x] Unit test: two clients. A project of client A gets A's client knowledge and never B's; a client-less project gets none
- [x] Update [docs/database.md](docs/database.md), [docs/apps.md](docs/apps.md), [docs/README.md](docs/README.md)

**Prerequisite:** every repo needs a pushable remote. Create the GitHub repo for `guest-management-fe` before registering it.

**Done when:** project `guest-management` has `backend` (go) and `frontend` (node) repos, and `go-sdk` is its own project (client-less, or both under a client if Guest Management is client work). A chain of go-sdk task → backend task → frontend task runs in order, and each goes through its own merge gate. A task in a client project loads that client's knowledge. A backend task and a frontend task (the frontend one `-after` the backend one) each go through the merge gate and land on their own repo's `main`.

---

### M1.9 V1 polish (found while running real tasks)
- [x] Edit a repository: `PATCH /projects/{ref}/repositories/{name}` (URL, branch, stack, test command), audited with before/after. CLI `empire repo set`
- [x] Edit a project: `PATCH /projects/{ref}` (name, autonomy level), audited. CLI `empire project set`
- [x] Record the agent's own summary on `agent_runs.summary`
- [x] Show the agent summary in the merge approval request, above the diffstat

**Done when:** you can change a repo's test command and a project's autonomy from the CLI, both show up in `audit_log`, and `empire approvals` shows what the agent says it changed.

---

## V2 — Persistent AI Interface (§41) — *deferred*
> Not needed yet. The `empire` CLI is the interface for now. V3 does not depend on V2. Revisit when you want phone access, notifications, or a remote worker.

**Goal:** manage development from phone or laptop without operating the infrastructure yourself.

### M2.1 MCP server over the Control Plane API (§5, §33)
- [ ] Expose the §33 operations as MCP tools (thin wrappers over the HTTP API, with no direct DB access)
- [ ] Give Hermes its own API token so the audit log shows `actor=hermes`
- [ ] Hermes can't approve on its own. Approval tools require a human-confirmed action (e.g. Hermes relays your explicit "approve", and the decision records `decided_by=owner via hermes`)

**Done when:** from an MCP client you can list projects, create a task, and approve a merge.

### M2.2 Hermes operator
- [ ] Pick the Hermes runtime (e.g. Claude with the MCP server attached, via a chat app or bot)
- [ ] Natural-language task creation: "add X to project Y" → `create_task` (project resolved by name)
- [ ] "What is currently running?" → a concise summary from `list_tasks` + `list_workers` (§36)
- [ ] "Why did…?" → read the `agent_run` log and linked docs
- [ ] Hermes memory stores only personal/operational notes, never task state (§31)

**Done when:** you complete the V1 demo entirely through Hermes from your phone.

### M2.3 Notifications (§39)
- [ ] One outbound channel (Telegram, Slack, email, or ntfy; pick one)
- [ ] Emit on: approval required, task failed, worker stale, repeated test failures, work completed
- [ ] Notification = an outbox table plus a sender loop, so a restart doesn't lose messages

**Done when:** an approval request pings your phone within seconds.

### M2.4 Remote worker
- [ ] Run the worker on a Mac Mini / mini PC against the remote control plane
- [ ] Per-worker token; secure transport (Tailscale/WireGuard is simplest)
- [ ] Worker gets only the repo credentials for the projects it's allowed to serve (§34)

**Done when:** a task gets claimed and completed by the remote machine while your laptop is off.

---

## V3 — AI Software Organization (§41)
**Goal:** the full SDLC runs through AI, stopping at human gates.

User guide: [docs/documents.md](docs/documents.md). Reference: [docs/workflows.md](docs/workflows.md).

### M3.1 Document contracts & templates (§16, §17, §19)
- [x] Contract + template for: PRD, Technical Design, ADR, API Spec, Database Design, Test Plan
- [x] Also shipped (client hand-over set): UX Spec, Architecture Overview, Implementation Plan, Deployment Plan, Runbook, Change Request, Release Notes, plus the internal types Guideline and Role
- [ ] *(defer)* Research Doc. Add it when a workflow first needs it
- [x] A contract lives as a YAML file ([knowledge/contracts/](knowledge/contracts/)) next to its template ([knowledge/templates/](knowledge/templates/)): required metadata, required sections, allowed relationships, upstream rules, approval requirement
- [x] Templates follow industry references (MADR, arc42/C4, ISO/IEC/IEEE 29119-3, RFC 9457, MoSCoW, ISO 25010); guidance lives in `<!-- -->` comments and placeholders are `{{ … }}`
- [x] A test fills every template and validates it against its contract, so templates and contracts cannot drift apart

### M3.2 Document validator (§18)
- [x] `empire docs validate [-project P]` (control plane, full checks) covering front matter, required sections, empty sections, placeholders, unique IDs, scope/folder, allowed relationship types, referenced IDs exist and are visible, status/version/dates, broken links and wikilinks
- [x] Upstream checks: e.g. a Technical Design's `satisfies` PRD must be approved before the design goes for approval
- [x] Wired into the workflow: invalid output goes back to the generating agent with the problem list (up to 3 attempts)
- [x] `empire docs validate -local` / `make docs-check` (offline), a pre-commit hook (`make hooks`), and a CI workflow ([.github/workflows/ci.yml](.github/workflows/ci.yml))

**Done when:** a broken document is rejected with a clear error list, and a valid one passes. ✅

### M3.3 Artifact versioning & change requests (§12)
- [x] Approving a document records `(scope, doc_id, version, content hash)` in the append-only `document_approvals` table. *Decision:* a content hash instead of a git sha — it works whether or not the knowledge folder is committed, and it ignores status changes and generated blocks
- [x] Guard: an approved version whose content changed → validation error; a new version needs `derived_from` an approved change request that `affects` it
- [x] Change-request flow (`change` workflow): CR → impact analysis at the gate → revise tasks (same id, version n+1, `derived_from: [CR]`) → approval. `supersedes` is used when a *different* document replaces an old one
- [x] Documents edited after submission cannot be approved until resubmitted

**Done when:** editing an approved PRD in place fails validation, and a proper v2 goes through approval. ✅

### M3.4 Workflow engine (§7)
- [x] Workflow definition as data ([workflows/](workflows/)): ordered steps with `kind`, `role`, `doc_type`, `inputs`, `relations`, `gate`, `review`
- [x] Ship `feature` (PRD → design → plan → code), `quick-fix` (code → AI review → merge gate), and `change` (CR → revise → plan → code)
- [x] Engine: a step's output is validated → gate → on approval the next step starts; rejection or cancellation cancels the request and withdraws its unapproved documents
- [x] The planner's work breakdown becomes N code tasks with dependencies, each in its own repository

**Done when:** "add QR ticket validation" produces PRD → gate → design → gate → tasks → code → merge gate. ✅

### M3.5 Agent roles (§26)
- [x] One file per role under [knowledge/roles/](knowledge/roles/): mission, responsibilities, inputs, outputs, allowed actions, not allowed, quality bar. Loaded into every task of that role
- [x] Product Manager, Architect, Planner, Developer, Reviewer
- [ ] *(defer)* Tester, Documentation, DevOps. Add each when a workflow step needs it
- [x] Enforced: the reviewer is a separate, read-only agent run; its REQUEST_CHANGES sends the task back (max 2 rounds); approval never comes from an agent (the owner may approve documents they wrote themselves)

### M3.6 Knowledge graph & traceability (§14, §15, §38)
- [x] Index: front matter → `documents` + `doc_edges`, rebuilt on every platform write, at control-plane start, by `empire docs reindex`, and by the post-commit hook
- [x] Tasks → documents (context and output) and commits → tasks (`merge_sha`, `Task: N` trailer, changed files)
- [x] `empire trace <DOC | task:N | request:N | commit:SHA | source file>` shows why something exists and what depends on it
- [x] Context resolver v2: a task's listed documents are expanded along the graph (satisfies, implements, depends_on, derived_from, affects, tested_by, and specs that implement a design)

**Done when:** "Why does this code exist?" answers with commit → task → design → PRD → original request. ✅

### M3.7 Obsidian (§20)
- [x] A generated "Relations" block of `[[wikilinks]]` at the end of each document, so Obsidian's graph shows the typed relationships
- [x] The repository stays fully usable without Obsidian (validator and index read front matter; the block is excluded from hashes and checks)

### M3.8 Change impact analysis (§37)
- [x] `empire impact <DOC>` = graph walk → affected documents (with the relationship path), tasks, merge commits, and changed code files per repository
- [x] Submitting a change request puts the impact report into its approval request automatically

**Done when:** changing a PRD shows the list of affected designs, APIs, tests, and code before you approve. ✅

### M3.9 Client hand-over
- [x] `empire docs new` scaffolds a document from its template with the next free id
- [x] `empire docs export -project P` writes approved documents without front matter or guidance, with a Document Control table and an index README

---

## V4 — Autonomous Software Factory (§41)
**Goal:** many projects running concurrently, with strict boundaries.

Do these in order of real pain, not in list order.

- [ ] **Multiple workers:** concurrency limits per worker/project; re-evaluate Redis here
- [ ] **Docker-per-task sandboxes:** filesystem and network boundaries, secrets injected per task (§34)
- [ ] **Multiple models + routing (§32):** add a second `Agent` implementation; route by a simple rule table (task type → model)
- [ ] **Cost tracking & budgets:** per-client and per-project token/cost totals, and budget caps that pause work
- [ ] **Advanced scheduling:** priorities, dependency-aware ordering, fair share across projects
- [ ] **Learning system (§23):** `learning_candidates` table → human review → promote to `stacks/` or `global/` (always gated)
- [ ] **Observability (§36):** a small web dashboard, or Grafana over Postgres; failures, retries, durations, costs
- [ ] **Automated knowledge indexing:** reindex on push via webhook
- [ ] **Advanced policy engine:** only if the YAML map from M1.3 becomes unmanageable
- [ ] *(defer)* vector search. Only if graph + folder scoping demonstrably fails to find context

---

## Cross-cutting (start early, keep going)

### Security (§34)
- [ ] Separate tokens for owner, Hermes, and each worker; per-token scopes
- [ ] Secrets never go into `.empire-context.md` or agent logs; scrub logs
- [ ] Protected branches on the remote (GitHub/GitLab) as a second line of defense
- [ ] Production deploy credentials are never available to workers without an approved gate

### Backups (§25)
- [ ] Knowledge repo: push to 2 remotes + a scheduled local mirror
- [ ] Postgres: nightly `pg_dump` with retention
- [ ] **Restore drill:** restore both into a fresh environment, once a quarter. Add it to your calendar

### Testing
- [x] Unit tests for the state machine, policy, context isolation, validator, workflows, graph expansion
- [x] End-to-end tests with a fake agent: V1 ([e2e_test.go](internal/worker/e2e_test.go)) and V3 workflows ([e2e_v3_test.go](internal/worker/e2e_v3_test.go))

---

## Milestone summary

| # | Milestone | Demo |
|---|-----------|------|
| M0 | Foundations | Stack runs, knowledge folder opens in Obsidian |
| M1.1–1.3 | Data, API, approvals | Task moves through states; gate blocks until approved |
| M1.4–1.6 | Git, context, worker | Crash-safe worker in isolated worktrees |
| M1.7 | Coding agent | **V1: task → code → tests → merge approval** |
| M1.8 | Clients & multi-repo projects | Client → projects → repos; client knowledge isolated per client |
| M1.9 | V1 polish | Edit repos/projects; agent summary in approvals |
| M2 | Hermes + notifications *(deferred)* | **V2: run it all from your phone** |
| M3.1–3.4 | Contracts, validation, workflows ✅ | **PRD → design → tasks pipeline with gates** |
| M3.5–3.9 | Roles, graph, impact, hand-over ✅ | **V3: traceability + impact analysis** |
| V4 | Factory | Multiple projects in parallel, within budget |
