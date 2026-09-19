# Apps Reference

The programs: what each one does, how to configure it, and how they talk. For the big picture, read [README.md](README.md) first. For documents and workflows from a user's point of view, see [documents.md](documents.md) and [workflows.md](workflows.md).

| App | Source | Binary | Role |
|-----|--------|--------|------|
| Control plane | [cmd/controlplane](../cmd/controlplane/main.go) → [internal/controlplane](../internal/controlplane/) | `bin/controlplane` | HTTP API, owns the database, runs workflows, guards documents |
| Worker | [cmd/worker](../cmd/worker/main.go) → [internal/worker](../internal/worker/) | `bin/worker` | Executes code and document tasks with git + Claude Code |
| CLI | [cmd/empire](../cmd/empire/main.go) | `bin/empire` | Your remote control |
| MCP server | [cmd/empire-mcp](../cmd/empire-mcp/main.go) → [internal/mcp](../internal/mcp/mcp.go) | `bin/empire-mcp` | Hermes' tools: the API as MCP tools, with the Hermes token (see [§6](#6-v2-hermes-notifications-remote-workers)) |

Build them all with `make build`. All config comes from environment variables. `make run-*` loads `.env` automatically, but the binaries don't, so load `.env` into your shell before running them directly. On Windows, run `. .\env.ps1`.

---

## 1. Control plane

### What it does

- Serves the HTTP API on `CP_ADDR` (default `:8787`).
- Is the **only** program with database access, and the only program that writes to `knowledge/`.
- Enforces:
  - **Authentication:** owner token vs worker token.
  - **Task state machine:** [internal/task/state.go](../internal/task/state.go).
  - **Policy:** which actions need a human ([internal/policy/policy.go](../internal/policy/policy.go)).
  - **Approval rules:** only a human decides; an AI requester never decides its own request.
  - **Task ownership:** a worker can only touch tasks it currently holds.
  - **Document rules:** contracts, validation, approved-version immutability ([internal/docs](../internal/docs/)).
- Runs the **workflow engine**: starts each step's tasks when the previous step is complete, and turns an approved implementation plan into code tasks.
- Builds each agent's context: the knowledge bundle, expanded along the document graph, plus templates, contracts, and drafts for document tasks.
- Keeps the **knowledge graph index** and the Obsidian relation links up to date. Reindexing happens at start-up and after every document change it makes.
- Answers **trace** and **impact** questions.
- Writes an `audit_log` row for every change, in the same transaction.
- Runs the **reaper** in the background every `EMPIRE_STALE_AFTER / 2`. It marks silent workers `offline` and requeues their tasks.

### Configuration

| Variable | Default | Meaning |
|----------|---------|---------|
| `DATABASE_URL` | `postgres://empire:empire@localhost:5433/empire?sslmode=disable` | Postgres connection |
| `EMPIRE_OWNER_TOKEN` | *(required)* | Bearer token for you |
| `EMPIRE_WORKER_TOKEN` | *(empty)* | Token shared by workers that have no token of their own. Empty = only per-worker tokens (`empire worker add`) |
| `EMPIRE_HERMES_TOKEN` | *(empty = Hermes disabled)* | Token for Hermes / `empire-mcp` |
| `EMPIRE_NTFY_URL` | *(empty = no notifications)* | ntfy topic URL, e.g. `https://ntfy.sh/<long-random-topic>` |
| `EMPIRE_NTFY_TOKEN` | *(empty)* | ntfy access token, for a protected topic |
| `CP_ADDR` | `:8787` | Listen address |
| `EMPIRE_KNOWLEDGE_DIR` | `knowledge` | Root of the knowledge repository (contracts and templates are read from here) |
| `EMPIRE_WORKFLOWS_DIR` | `workflows` | Workflow definitions. Checked at start-up; restart after editing |
| `EMPIRE_STALE_AFTER` | `60s` | A worker with no heartbeat for this long is considered dead |

It refuses to start if the owner token is missing, two tokens are equal, the database is unreachable, a contract can't be read, or a workflow is invalid.

### Code layout

| File | Contents |
|------|----------|
| [server.go](../internal/controlplane/server.go) | Routes, auth, error mapping, JSON helpers, transaction and audit helpers, start-up loading of workflows |
| [projects.go](../internal/controlplane/projects.go) | Clients, projects (create, edit, move between clients), repositories, `scopeFor()` (which knowledge a task may see) |
| [tasks.go](../internal/controlplane/tasks.go) | Stand-alone tasks, cancel/retry (`cancelTaskTx` withdraws unapproved documents), `move()` (the single status-change function) |
| [workers.go](../internal/controlplane/workers.go) | Worker protocol: register, heartbeat, claim, transition, authorize, task context, runs. Also the reaper |
| [approvals.go](../internal/controlplane/approvals.go) | List approvals; `decide()` for merge gates and documents |
| [requests.go](../internal/controlplane/requests.go) | Workflow engine: requests, `advance()`, step start, plan → code tasks, document upload (`taskOutput`), AI review verdicts |
| [documents.go](../internal/controlplane/documents.go) | Document list, new, validate, submit, approve/reject (`decideDocuments`), export |
| [knowledge.go](../internal/controlplane/knowledge.go) | Reading and writing `knowledge/` safely, staged writes with rollback, reindex, graph-based context expansion |
| [graph.go](../internal/controlplane/graph.go) | Impact analysis and trace |

### HTTP API

Every request needs `Authorization: Bearer <token>`. Worker calls also send `X-Worker-ID: <id>`. Errors come back as `{"error": "..."}`.

Three roles: the **owner**; **Hermes**, which may read everything and create/cancel/retry tasks and requests, read the audit log, and relay decisions (with a confirm code, see §6), but not manage clients, projects, repositories, documents or workers; and **workers**, which may only use the worker protocol and cannot read the rest of the API.

`{ref}` means a slug or a numeric id: `/projects/guest` and `/projects/3` are the same. This works because slugs always contain a letter.

| Status code | Meaning |
|-------------|---------|
| 400 | Bad input |
| 401 | Bad token |
| 403 | Right token, wrong role, or not allowed |
| 404 | Not found |
| 409 | Illegal state change, already decided, duplicate slug/name, a client move that would split dependencies, or a document changed since submission |
| 422 | Document is not valid (the message lists every problem) |

**Owner endpoints: organization and tasks**

| Method & path | Body | Does |
|---------------|------|------|
| `POST /clients` | `{slug, name, default_autonomy_level?}` | Create a client |
| `POST /projects` | `{slug, name, client?, autonomy_level?}` | Create a project. Autonomy defaults to the client's default, else `conservative` |
| `PATCH /projects/{ref}` | `{name?, autonomy_level?}` | Edit a project. Audited with before/after |
| `POST /projects/{ref}/client` | `{client}` (slug, or `""` for none) | Move a project to another client. Refused (409) if it would split a dependency chain across clients. Autonomy is unchanged |
| `POST /projects/{ref}/repositories` | `{name, repo_url, stack, default_branch?, test_command?}` | Add a repository |
| `PATCH /projects/{ref}/repositories/{name}` | `{repo_url?, default_branch?, stack?, test_command?}` | Edit a repository (omitted fields are unchanged; `test_command: ""` clears it). Takes effect from the next claim. Audited with before/after |
| `POST /tasks` | `{project, repository?, title, description?, required_capabilities?, context_docs?, depends_on?}` | Create a stand-alone code task. `repository` (name) may be omitted only if the project has one repo. `depends_on` may point to any project **of the same client** (or client-less to client-less). Checks that `context_docs` exist |
| `POST /tasks/{id}/cancel` | none | → `CANCELLED`; closes open gates; withdraws unapproved documents; cancels its request |
| `POST /tasks/{id}/retry` | none | `FAILED` → `PENDING`, resets attempts |
| `GET /audit?target=task:1` | none | Last 200 audit rows (all, or for one target) |

**Owner endpoints: workflows and decisions**

| Method & path | Body | Does |
|---------------|------|------|
| `POST /requests` | `{project, workflow?, repository?, title, description?}` | Start a request (`workflow` defaults to `feature`). `repository` is required for workflows that start with code when the project has several repos |
| `POST /requests/{id}/cancel` | none | Cancel the request, its open tasks and gates; withdraw its unapproved documents |
| `POST /approvals/{id}/approve` | `{comment?}` | Merge gate: task → `PENDING` (resumes to merge). Document: records the approved version(s), sets `approved`, completes the task, advances the request |
| `POST /approvals/{id}/request-changes` | `{comment}` (required) | Merge gate: back to implement with your feedback. Document: `changes_requested`, the agent reworks the draft |
| `POST /approvals/{id}/reject` | `{comment?}` | Task → `CANCELLED`, documents → `rejected`, request → `cancelled` |

**Owner endpoints: documents**

| Method & path | Body | Does |
|---------------|------|------|
| `POST /documents` | `{project, type, title, owner}` | Create a document from its template with the next free id |
| `POST /documents/submit` | `{ref, project?}` | Validate a human-written document and open its approval request. `ref` = knowledge path, key (`projects/x/PRD-001`), or id with `project` |
| `POST /documents/reindex` | none | Rebuild the graph index and Obsidian relation links from the files |
| `GET /documents/export?project=&all=` | none | Client-ready documents: `[{name, content}]`, a README index first. Approved only unless `all=true` |

**Read endpoints (owner or worker)**

| Method & path | Returns |
|---------------|---------|
| `GET /clients`, `GET /clients/{ref}` | Clients |
| `GET /projects`, `GET /projects/{ref}` | Projects |
| `GET /projects/{ref}/repositories`, `GET /repositories` | Repositories of one project, or all |
| `GET /tasks?status=&project_id=` | Up to 500 tasks, newest first |
| `GET /tasks/{id}` | `{task, runs, approvals}` |
| `GET /approvals?status=`, `GET /approvals/{id}` | Approval requests |
| `GET /workers` | Workers |
| `POST /workers` | `{name, projects?}`: give a worker its own token (shown once), limited to those projects. Again = rotate. Owner only |
| `GET /workflows` | Workflow names, descriptions, steps |
| `GET /requests?project=&status=`, `GET /requests/{id}` | Requests; detail = `{request, steps, tasks, documents, approvals}` |
| `GET /documents?project=&type=` | Documents with status, version, and whether the current content is approved |
| `POST /documents/validate` | `{project?}` → list of `{path, message}` problems (full checks, with approval records) |
| `GET /trace?ref=&project=` | `{subject, why, effects}`. `ref` = `task:N`, `request:N`, `commit:SHA`, a document key/path, or an id with `project` |
| `GET /impact?doc=&project=` | `{document, documents, tasks, report}` |
| `GET /healthz` | `ok` (no auth) |

**Worker endpoints.** Task endpoints only work on a task the calling worker currently holds.

| Method & path | Body | Does |
|---------------|------|------|
| `POST /workers/register` | `{name, capabilities}` | Upsert by name, return the worker. **Requeues anything it held before** |
| `POST /workers/{id}/heartbeat` | `{task_id?}` | Refresh liveness. Replies `{cancel: true}` if the task is no longer the worker's |
| `POST /workers/{id}/claim` | none | Next matching `PENDING` task: `{task, project, repository}` (repository empty for document tasks), or `204` |
| `POST /tasks/{id}/transition` | `{to, error?, merge_sha?, changed_files?}` | Allowed `to`: `RUNNING`, `TESTING`, `REVIEWING`, `COMPLETED` (code tasks in the merge stage only, with the merge commit and changed files), `FAILED`, `PENDING` |
| `POST /tasks/{id}/authorize` | `{action, stage, summary?, subject_version?}` | Policy check. Replies `{allowed, approval_id?, subject_version?}`. If a human is needed, opens a gate and parks the task |
| `GET /tasks/{id}/context` | none | `{files, content, kind, role, doc_type, templates, contracts, request, repositories, drafts, relations}` |
| `POST /tasks/{id}/runs` | `{model, context_files, role}` | Start an agent run, return `{id}` |
| `POST /runs/{id}/finish` | `{exit_status, log_path, tokens, cost_usd, summary}` | Close the run. `summary` is what the agent says it did |
| `POST /tasks/{id}/output` | `{files: [{name, content}], summary}` | Document tasks: validate and store the documents. Replies `{accepted, problems?, documents?, status?}`. Up to 10 files of 512 KB each |
| `POST /tasks/{id}/review` | `{verdict, summary}` | AI review result. Replies `{rework}`: `true` sends the task back to the developer (max 2 rounds) |

**How claiming works:** a single SQL query picks the lowest-id `PENDING` task that meets all of these conditions:

- its required capabilities ⊆ the worker's capabilities (document tasks require none)
- all its dependencies are `COMPLETED`
- the worker is `online`

It uses `FOR UPDATE SKIP LOCKED`, so two workers can never claim the same task.

### What happens to an uploaded document

1. **Parse.** The platform identifies the main document (the task's `doc_type`) and any allowed extras (e.g. ADRs from the design step). Anything else is refused.
2. **Platform fields.** It sets what the platform owns:
   - `id`: the next free number, or the document's existing id;
   - `project`;
   - `version`: unchanged for rework; approved + 1 for revisions;
   - `status`: `pending_approval`, or `draft` for plans without a gate;
   - `owner` (if missing), `created`, `updated`;
   - the links the workflow guarantees, e.g. `satisfies: [PRD-001]`, `derived_from: [CR-001]`, and the design `depends_on` its ADRs.
3. **Write and validate.** It writes the files, then validates them against the repository. On problems it restores the files and returns the list to the agent.
4. **Open the gate.** It records `output_docs`, reindexes, opens one approval request covering all the documents, and parks the task. Plans without a gate create their code tasks immediately.

---

## 2. Worker

### What it does

On start: **register**, then loop forever. The loop is **claim** a task → execute it → repeat, with a poll every 5s when idle. A separate **heartbeat** runs every 10s.

**Code tasks**

```text
claim ──► RUNNING ──► ensure clone  workspaces/<project>/<repo>/_base   (clone --no-checkout, then fetch)
                          │
            ┌─────────────┴──────────────┐
      stage=implement                stage=merge
            │                              │
  worktree …/<repo>/task-N            authorize merge_protected
  on branch ai/task-N                   (must already be APPROVED → gives sha)
  (continues the pushed branch          worktree (detached) at origin/main
   if it exists)                        git merge --no-ff <approved sha>
            │                           TESTING: test_command
  GET context → .empire-context.md      git push origin HEAD:main
  developer run (claude -p)             COMPLETED + merge sha + changed files
            │
  TESTING: test_command
  git add -A, commit "title\n\nTask: N"
  (fails if nothing changed)
  authorize push_branch → git push ai/task-N
            │
  if review: REVIEWING → reviewer run (read-only) on the diff
            ├─ REQUEST_CHANGES (rounds < 2) → task back to PENDING with the review as feedback
            └─ otherwise → continue
  authorize merge_protected
     → gate opened (agent summary + AI review + diffstat), task parked (WAITING_FOR_HUMAN)
            │
  remove worktree
```

**Document, plan, and revise tasks**

```text
claim ──► RUNNING ──► scratch folder workspaces/<project>/_docs/task-N/
                        .empire-context.md       knowledge bundle (role, rules, input documents + linked ones)
                        .empire/job.json         task, request, repositories, relations
                        .empire/templates/*.md   template(s) for the document type(s)
                        .empire/contracts/*.yaml contract(s)
                        output/                  previous draft or the document being revised
          ┌──────────────► agent run in its role (product-manager / architect / planner)
          │                  │
          │               POST output/*.md
          │                  ├─ accepted → WAITING_FOR_HUMAN (or COMPLETED for an ungated plan)
          └── problems ◄─────┘  (up to 3 attempts, then FAILED with the problem list)
```

Any error → `FAILED` with the error text (last 4000 chars) in `last_error`.

### Configuration

| Variable | Default | Meaning |
|----------|---------|---------|
| `EMPIRE_CP_URL` | `http://localhost:8787` | Control plane address |
| `EMPIRE_WORKER_TOKEN` | *(required)* | The control plane's shared worker token, or this worker's own token from `empire worker add` |
| `EMPIRE_WORKER_NAME` | hostname | Identity. Reusing a name reuses the worker row |
| `EMPIRE_WORKER_CAPS` | `go` | Comma-separated capabilities, e.g. `go,node` |
| `EMPIRE_WORKSPACES` | `workspaces` | Where clones, worktrees, scratch folders and logs go |
| `EMPIRE_AGENT` | `claude` | `claude` = real Claude Code. `fake` = predictable output for every task kind (free, for testing and dry runs) |
| `EMPIRE_AGENT_MODEL` | *(empty)* | Passed as `claude --model` |

### Files on disk

```text
workspaces/
├── <project slug>/
│   ├── <repo name>/
│   │   ├── _base/       one clone per repository (.git/info/exclude hides .empire-context.md and .empire/)
│   │   └── task-7/      worktree while task 7 runs (deleted afterwards)
│   └── _docs/
│       └── task-9/      scratch folder while document task 9 runs (deleted afterwards)
└── logs/
    └── task-7-run-3.log full agent output (JSON from claude -p), one file per run
```

### The agent (Claude Code)

It's run as `claude -p --output-format json --permission-mode <mode> [--model M]`:

- **Mode:**
  - `acceptEdits` for developers and document writers: they can edit files but can't run shell commands;
  - `plan` (read-only) for the reviewer.
- **Where it runs:** the task worktree or scratch folder is its working directory, and the prompt arrives on stdin.
- **Environment:** every `EMPIRE_*` variable and `DATABASE_URL` are removed, so it can't call the control plane.
- **What it's told:**
  - its role (the definition itself is in the context bundle);
  - the task and the original request;
  - your feedback or the validation problems from its last attempt;
  - the writing rules for client-facing documents;
  - never to touch git, and to raise concerns about approved requirements instead of silently changing them.

  Prompts are in [prompts.go](../internal/worker/prompts.go).
- **What gets recorded:** tokens, cost, and the agent's final summary are parsed from its JSON output.

To add another agent, implement the `Agent` interface in [agent.go](../internal/worker/agent.go) (`Name()` and `Run(ctx, Job)`, where `Job` has `Dir`, `Prompt`, `Log`, `ReadOnly`), then add it to the switch in [cmd/worker/main.go](../cmd/worker/main.go).

### The fake agent

`EMPIRE_AGENT=fake` runs every workflow without calling an AI:

- **Code:** appends a line to `empire-fake-agent.txt`.
- **Review:** approves. It requests changes once if the task title contains `[rework]`.
- **Documents:**
  - fills the template with "Example" text, and adds one ADR when the step allows extras;
  - a plan gets one work item per repository, chained;
  - a change request affects the ids after `affects:` in the request description;
  - rework or revision appends a line to the existing draft.

### Code layout

| File | Contents |
|------|----------|
| [worker.go](../internal/worker/worker.go) | Register, heartbeat, `Step` (claim + execute), implement / review / merge, runs, tests |
| [document.go](../internal/worker/document.go) | Document task execution: scratch folder, job file, upload and retry |
| [prompts.go](../internal/worker/prompts.go) | Developer, reviewer, and document prompts |
| [git.go](../internal/worker/git.go) | Command runner, clone/fetch, worktree add/remove, git excludes |
| [agent.go](../internal/worker/agent.go) | `Agent` interface, `ClaudeCode`, `Fake` |
| [e2e_test.go](../internal/worker/e2e_test.go), [e2e_v3_test.go](../internal/worker/e2e_v3_test.go) | Full flows against real Postgres + git |

---

## 3. CLI (`empire`)

It uses `EMPIRE_CP_URL` (default `http://localhost:8787`) and `EMPIRE_OWNER_TOKEN`. **Flags go before positional arguments.**

On Windows, run `. .\env.ps1` in the terminal first ([env.ps1](../env.ps1)). It sets both variables and adds `bin\` to PATH.

**Organization and tasks**

| Command | Example |
|---------|---------|
| Create client | `empire client create -slug acme -name "Acme Corp" -autonomy conservative` |
| List clients | `empire client list` |
| Create project | `empire project create -slug shop -name "Shop" -client acme` (omit `-client` for personal projects; `-autonomy` overrides the client default) |
| List projects | `empire project list` (shows client and repositories) |
| Edit project | `empire project set shop -autonomy medium` · `empire project set shop -name "Shop v2"` |
| Move project | `empire project move shop -client other` · `empire project move shop -none` (autonomy is not changed; use `project set`) |
| Add repository | `empire repo add -project shop -name backend -repo git@github.com:me/shop-be.git -stack go -test "go test ./..."` (`-branch` defaults to `main`) |
| Edit repository | `empire repo set -project shop -name backend -test "go test -short ./..."` · `-branch develop` · `-repo URL` · `-stack go` · `-test ""` clears it |
| List repositories | `empire repo list` · `empire repo list -project shop` |
| Create task | `empire task create -project shop -repo backend -desc "details…" -doc requirements/PRD-001-x.md -after 3 Add QR validation` |
| List tasks | `empire task list` · `empire task list -status FAILED` · `empire task list -project shop` (shows kind and role) |
| Task detail | `empire task get 5` (JSON: task + runs + approvals) |
| Cancel / retry | `empire task cancel 5` · `empire task retry 5` |
| Workers | `empire workers` |
| Audit | `empire audit` · `empire audit -target task:5` · `empire audit -target document:projects/shop/PRD-001` |

**Workflows and decisions**

| Command | Example |
|---------|---------|
| List workflows | `empire workflows` |
| Start a request | `empire request create -project shop -desc "…" Add QR validation` · `-workflow quick-fix -repo frontend` · `-workflow change` |
| List requests | `empire request list` · `-project shop` · `-status active` |
| Request detail | `empire request get 3` (steps, tasks, documents, what waits for you) |
| Cancel a request | `empire request cancel 3` |
| Pending gates | `empire approvals` (default `-status PENDING_APPROVAL`) |
| Decide | `empire approve 2 -m "ok"` · `empire request-changes 2 -m "add tests"` · `empire reject 2` |

**Documents and the knowledge graph**

| Command | Example |
|---------|---------|
| New document | `empire docs new -project shop -type prd -title "Loyalty points" -owner "Bia, Product"` |
| List documents | `empire docs list -project shop` · `-type adr` |
| Validate | `empire docs validate -project shop` (full) · `empire docs validate -local` (offline, no control plane) |
| Submit | `empire docs submit PRD-001 -project shop` · `empire docs submit knowledge/projects/shop/requirements/PRD-001-x.md` |
| Reindex | `empire docs reindex` |
| Export | `empire docs export -project shop` (→ `exports/shop`) · `-out DIR` · `-all` |
| Trace | `empire trace PRD-001 -project shop` · `empire trace commit:3f2a9c1` · `empire trace task:12` · `empire trace request:3` · `empire trace path/to/source.go` (reads `Task:` trailers from that file's git history) |
| Impact | `empire impact PRD-001 -project shop` |

`task create` flags:

| Flag | Meaning |
|------|---------|
| `-project` | Project slug or id (required) |
| `-repo` | Repository name. Required if the project has more than one |
| `-desc` | Description sent to the agent |
| `-doc` | Repeatable. A file under `knowledge/projects/<slug>/` to include in context (the documents it links to are added automatically) |
| `-cap` | Repeatable. Required worker capability (default: the repository's stack) |
| `-after` | Repeatable. A task id that must be `COMPLETED` first. May be in another project of the **same client** |

### Example: a feature across repositories, by hand

```sh
empire client create -slug acme -name "Acme"
empire project create -slug sdk   -name "Go SDK" -client acme
empire project create -slug guest -name "Guest Management" -client acme
empire repo add -project sdk   -name sdk      -repo https://github.com/me/go-sdk.git -stack go   -test "go test ./..."
empire repo add -project guest -name backend  -repo https://github.com/me/guest-be.git -stack go  -test "go test ./..."
empire repo add -project guest -name frontend -repo https://github.com/me/guest-fe.git -stack node -test "npm test"

empire task create -project sdk "add QR token helper"                                   # → task 1
empire task create -project guest -repo backend  -after 1 "validate QR tickets"        # → task 2
empire task create -project guest -repo frontend -after 2 "QR scanner screen"          # → task 3
```

The tasks run in order 1 → 2 → 3, each after the previous one is merged. You get one merge gate per repository. Within one project, `empire request create` does all of this for you: the planner writes the tasks and their order.

---

## 4. Shared packages

| Package | Used by | Contents |
|---------|---------|----------|
| [internal/api](../internal/api/types.go) | all | JSON structs for every request/response. Also the DB row mapping (`db` tags) |
| [internal/client](../internal/client/client.go) | worker, CLI | `Client.Do(method, path, in, out)`, which returns `*client.Error` on non-2xx responses |
| [internal/task](../internal/task/state.go) | control plane, worker | Status/stage constants, `CanTransition`, `InFlight`, `MaxAttempts = 3` |
| [internal/policy](../internal/policy/policy.go) | control plane, worker | Action names, risk sets, `Requires`, `CanDecide` |
| [internal/knowledge](../internal/knowledge/context.go) | control plane | `Resolve(root, Scope{Stacks, Client, Project, Role, Docs})`, which returns a `Bundle` |
| [internal/docs](../internal/docs/) | control plane, worker (fake agent), CLI (offline validation) | Document parsing and editing (front matter keeps order and comments), contracts, reference resolution with scope rules, validator, versioning checks, content hash, work-breakdown parsing, graph expansion, Obsidian relations block, client export |
| [internal/workflow](../internal/workflow/workflow.go) | control plane | Workflow loading and checks |

### Context bundle rules

What `knowledge.Resolve` includes, in order:

1. every `*.md` under `knowledge/global/`
2. `knowledge/roles/<role>.md` for the task's role
3. every `*.md` under `knowledge/stacks/<stack>/`: the repository's stack for code tasks, all project stacks for document tasks
4. every `*.md` under `knowledge/clients/<project's client>/`, **only if the project has a client**
5. the task's project documents from `knowledge/projects/<project slug>/`, which the control plane first expands along the graph (`docs.ExpandContext`, 3 hops, same project only, skipping superseded and rejected documents)

`README.md` files are skipped (they're folder navigation). The function never reads:

- other stacks
- other clients
- other projects
- any client folder at all, for client-less projects

Doc paths that try to escape the project folder (`..`, absolute paths, symlinks) are rejected by `os.OpenInRoot`. A dependency on a task in another project never adds that project's knowledge.

---

## 5. Tests

| Test | What it proves |
|------|----------------|
| [task/state_test.go](../internal/task/state_test.go) | Legal and illegal transitions |
| [policy/policy_test.go](../internal/policy/policy_test.go) | Presets, "high risk always asks", fail-closed behavior, no AI approval, owner may approve own documents |
| [knowledge/context_test.go](../internal/knowledge/context_test.go) | No leakage across stacks, clients or projects; roles; client-less projects get no client knowledge; path-escape rejection |
| [docs/docs_test.go](../internal/docs/docs_test.go) | **Every real template satisfies its contract**; the validator catches each class of problem; approved versions are immutable and v2 needs an approved change request; upstream approval; work-breakdown parsing; scope visibility; graph expansion; client export |
| [workflow/workflow_test.go](../internal/workflow/workflow_test.go) | The shipped workflows load; invalid definitions are refused |
| [worker/e2e_test.go](../internal/worker/e2e_test.go) `TestV1EndToEnd` | Implement → gate → request changes → rework → approve → merge of the approved sha; restart and heartbeat-loss requeue; dependencies; cancel; audit is complete and append-only |
| [worker/e2e_test.go](../internal/worker/e2e_test.go) `TestClientsAndRepositories` | Clients, repository rules, a cross-project chain with one gate per repo, context isolation, client moves, project/repository edits |
| [worker/e2e_v2_test.go](../internal/worker/e2e_v2_test.go) `TestV2EndToEnd` | Hermes over MCP creates a task (project by name) and is refused owner-only calls; workers can't read the API; a per-worker token can't be hijacked by the shared token and only claims its projects; the log tail reaches the control plane; the approval notification carries the confirm code; Hermes can't decide without it and is audited as `owner via hermes`; completion is notified |
| [worker/e2e_v3_test.go](../internal/worker/e2e_v3_test.go) `TestFeatureWorkflow` | The `feature` workflow end to end:<br>• PRD with rework<br>• design + ADR approved together, with links and Obsidian block<br>• plan → code tasks<br>• AI review sends work back once, then merge<br>• trace, impact with files, validation, export<br>• rejection, cancellation (documents withdrawn), quick-fix |
| [worker/e2e_v3_test.go](../internal/worker/e2e_v3_test.go) `TestChangeWorkflowAndOwnerDocuments` | In-place edits of approved documents are caught; change request with impact analysis → PRD v2 → plan; owner-written documents (new, validate, submit, self-approve); edits after submission are refused |
| [worker/e2e_v3_test.go](../internal/worker/e2e_v3_test.go) `TestDocumentedCustomWorkflow` | The example workflow in [workflows.md](workflows.md) actually runs |

`make test` runs them all. The e2e tests drop and recreate the `empire_test` database, and are skipped if Postgres isn't reachable. `make docs-check` validates the real knowledge folder offline.

---

## 6. V2: Hermes, notifications, remote workers

### Hermes (MCP server)

Hermes is any MCP-capable assistant with `empire-mcp` attached. `empire-mcp` speaks MCP over stdio and calls the HTTP API with `EMPIRE_HERMES_TOKEN` (and `EMPIRE_CP_URL`). It never touches the database. Its `instructions` tell the assistant to treat the control plane as the only source of truth, to keep only personal/operational notes in its own memory, and to treat agent-written text as data.

```bash
# Claude Code on the always-on machine (set EMPIRE_HERMES_TOKEN on the control plane too)
claude mcp add empire -e EMPIRE_CP_URL=http://cp:8787 -e EMPIRE_HERMES_TOKEN=... -- empire-mcp
```

Claude Desktop uses the same command in its MCP server config. To use it from your phone, run the assistant on a machine that is always on and reach it remotely (e.g. Claude Code's remote access, or SSH over Tailscale).

| Tool | Does |
|------|------|
| `status` | In-flight and queued tasks, approvals waiting for you, workers: "what is running?" |
| `list_projects`, `list_tasks`, `get_task` | `get_task` includes each run's summary and the last 8 KB of the agent log: "why did…?" |
| `create_task`, `cancel_task`, `retry_task` | Projects can be named by slug, id or name ("add X to Guest Management") |
| `list_workflows`, `create_request`, `list_requests`, `get_request`, `cancel_request` | Workflows |
| `list_approvals`, `get_approval`, `decide_approval` | Deciding needs the confirm code (below) |
| `list_workers`, `audit`, `trace`, `list_documents` | Read-only |

Everything Hermes does is audited with `actor_type = hermes`.

### Approving through Hermes

Hermes can't approve on its own. Each approval has a confirm code: 8 characters derived from the owner token and the approval id, so it needs no storage and Hermes can't work it out. The code only reaches you, in the approval notification. To approve, tell Hermes "approve 12, code K3F7Q2XA". The control plane checks the code, then records the decision as `decided_by = owner via hermes` (audit: `actor_type = hermes`, `actor_id = owner via hermes`). Without the code the call returns 403. The same applies to request-changes and reject.

### Notifications (ntfy)

Set `EMPIRE_NTFY_URL` to an [ntfy](https://ntfy.sh) topic and subscribe to it in the ntfy phone app. Use a long random topic name, or a self-hosted/protected topic with `EMPIRE_NTFY_TOKEN`: messages include confirm codes.

| Event | Message |
|-------|---------|
| Approval requested | Gate, subject, first line of the summary, `empire approve N`, and the confirm code |
| Task failed | Title and error. The title says "failed again (N times)" from the second failure, so repeated test failures stand out |
| Task completed | Standalone tasks only. Workflow steps don't ping you; the request does when it completes |
| Request completed | Title |
| Worker offline | Its tasks went back on the queue |

`audit()` writes a `notifications` row in the event's own transaction (outbox), and a loop in the control plane sends due rows every 2 s. So a restart never loses a message. Failed sends are retried with backoff (2^attempts seconds, at most 1 h). Anything still unsent after a day is dropped as `expired`.

### Remote worker

1. Reach the control plane over Tailscale/WireGuard (simplest), or put it behind a TLS reverse proxy. Don't expose `:8787` to the internet.
2. Give the machine its own token, limited to the projects it may serve:
   ```bash
   empire worker add -name mini -project guest-management -project go-sdk
   ```
3. On the machine, set `EMPIRE_CP_URL`, `EMPIRE_WORKER_NAME=mini`, and `EMPIRE_WORKER_TOKEN=<token>`, and start `worker`. Give it git credentials **only** for those projects' repositories. The control plane never hands it another project's task.

A worker token is bound to its name. It can't register under another name. The shared `EMPIRE_WORKER_TOKEN` can't take over a worker that has its own token. No worker token can read the owner API. Once every worker has its own token, clear `EMPIRE_WORKER_TOKEN`. Run `empire worker add` again with the same name to rotate a token.
