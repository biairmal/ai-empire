# Guest Management

Personal project with no client. The knowledge here was extracted on 2026-09-17 from `C:\Dev\Projects\Guest Management`. Every document is a **draft**: review each one, then `empire docs submit <ID> -project guest-management` and approve it.

## Documents

| ID | Document | Answers |
|----|----------|---------|
| PRD-001 | [Guest Management Platform](requirements/PRD-001-guest-management-platform.md) | What the product is, for whom; stories US-01–US-09 (B1–B14) |
| ARCH-001 | [Architecture Overview](architecture/ARCH-001-guest-management-architecture-overview.md) | Containers, components, cross-cutting concerns, risks |
| API-001 | [API Specification](api/API-001-guest-management-api-specification.md) | All 66 `/api/v1` operations, auth matrix, errors |
| DB-001 | [Database Design](database/DB-001-guest-management-database-design.md) | 17 tables, constraints, PII, migrations 000001–000019 |
| UX-001 | [Login and User Management](ux/UX-001-login-and-user-management.md) | B11 screens and the design-system foundations |
| ADR-001 | [Feature-sliced modular monolith on go-sdk](decisions/ADR-001-build-a-feature-sliced-modular-monolith-on-go-sdk.md) | |
| ADR-002 | [Tenant isolation in the application layer](decisions/ADR-002-enforce-tenant-isolation-in-the-application-layer.md) | |
| ADR-003 | [Stateless access and refresh JWTs](decisions/ADR-003-use-stateless-access-and-refresh-jwts.md) | |
| ADR-004 | [One role engine, system and event scopes](decisions/ADR-004-use-one-role-engine-with-system-and-event-scopes.md) | |
| ADR-005 | [Guest PII encryption and blind index](decisions/ADR-005-encrypt-guest-contact-data-with-a-blind-index.md) | |
| ADR-006 | [Invitations through a queue abstraction](decisions/ADR-006-publish-invitations-through-a-queue-abstraction.md) | |
| ADR-007 | [Soft delete; append-only scan logs](decisions/ADR-007-soft-delete-entities-and-keep-scan-logs-append-only.md) | |
| ADR-008 | [Seed events from templates in one transaction](decisions/ADR-008-seed-events-from-category-templates-in-one-transaction.md) | |
| ADR-009 | [Separate `/me` and admin routes](decisions/ADR-009-separate-self-service-me-routes-from-admin-routes.md) | |
| ADR-010 | [go-sdk release and development tags](decisions/ADR-010-consume-go-sdk-through-release-and-development-tags.md) | |
| GL-001 | [go-sdk versioning and consumption](guidelines/GL-001-go-sdk-versioning.md) | Loaded into every task |
| GL-002 | [Design system and design tooling](guidelines/GL-002-design-tooling.md) | Loaded into every task |
| GL-003 | [End-to-end testing with Postman and newman](guidelines/GL-003-end-to-end-testing.md) | Loaded into every task |

Suggested approval order: guidelines and ADRs, then PRD-001, then ARCH-001, DB-001, and API-001, then UX-001. UX-001 needs PRD-001 approved first.

## Registering the project

Owner decisions from 2026-09-17: autonomy is `high` (A3); `go-sdk` is a repository of this project (A4); every repository gates merges with its own `make check` (A2, C4).

```powershell
empire project create -slug guest-management -name "Guest Management" -autonomy high
empire repo add -project guest-management -name backend  -repo https://github.com/biairmal/guest-management-be.git -stack go   -test "make check"
empire repo add -project guest-management -name frontend -repo https://github.com/biairmal/guest-management-fe.git -stack node -test "make check"
empire repo add -project guest-management -name go-sdk   -repo https://github.com/biairmal/go-sdk.git            -stack go   -test "make check"
```

With `high` autonomy, medium-risk actions (schema, API, auth, dependency, infrastructure, and architecture changes) run without asking you. Document approvals and every merge still wait for you.

**Worker machine prerequisites:** Go 1.25, GNU make, and the tools that `make check` needs (`make install-tools` in `go-sdk` and in the backend: gofumpt, golangci-lint, govulncheck), network access for `govulncheck`, `bun` for the frontend, and Node/`npx` for newman.

**Don't run backend code tasks until the ADR-010 follow-ups are done** (commit and tag `go-sdk` `v0.1.0`, then remove `replace` from the backend's `go.mod`).

## Which workflow to use

| Work | Workflow |
|------|----------|
| New backend capability (for example B10, B13, B14) | `feature` |
| New screens (for example the guest list) | `feature-ui` (adds a UX Designer step) |
| End-to-end tests for built features | `e2e-tests -repo backend` (Tester; Postman/newman per GL-003) |
| Small fix | `quick-fix -repo <name>` |
| Change to an approved document | `change` |

The project guidelines in `guidelines/` are loaded into every task, so roles stay tool-agnostic and this project supplies its tools: GL-002 for design, GL-003 for end-to-end testing.

## Where the rest of the knowledge lives

- **Coding rules stay in the repositories.** Agents read them from the worktree: `guest-management-be/AGENTS.md` plus `docs/PATTERNS.md`, `NEW_FEATURE_CHECKLIST.md`, and `TESTING.md`; `guest-management-fe/AGENTS.md` (Next.js 16 warning) and `docs/DESIGN_SYSTEM.md`; `go-sdk/AGENTS.md`.
- **Not converted into documents:**
  - The per-feature technical designs for already-shipped B7, B8, B9, B11, and B12 (their decisions are in the ADRs and their behaviour in API-001 and DB-001; the full text stays in `DEVELOPMENT_PLAN.md`).
  - `go-sdk/docs/*` (a library, not this product).
  - `specs/REPOSITORY_SPEC.md` and `SQLKIT_SPEC.md` (go-sdk design specs from February 2026).
  - `AI_SOFTWARE_DEV_EMPIRE.md`, `AI_DEV_EMPIRE_ROADMAP.md`, and `AI_LEARNING_JOURNEY.md` (about AI Empire itself, not this product).
- **Not created because there is no source material:** Test Plan, Deployment Plan, Runbook, Release Notes. See B-questions.
- **Unbuilt features** B10 (post-event messages), B13 (event reports), and B14 (incidents) are stories in PRD-001. Start each one with `empire request create -project guest-management -desc "... depends_on: PRD-001" "<title>"`.

## Decisions (answered 2026-09-17)

| # | Answer | Recorded in |
|---|--------|-------------|
| A1 | `go-sdk` is consumed through tags, with development tags separate from release tags | ADR-010, GL-001 |
| A2 | Each repository's own `make check` | Registration commands above |
| A3 | Autonomy `high` | Registration commands above |
| A4 | `go-sdk` is a repository of this project | Registration commands above |
| A5 | New AI Empire roles `ux-designer` and `tester`, tool-agnostic. Tools are named per project in guidelines (GL-002, GL-003) | `knowledge/roles/`, `workflows/feature-ui.yaml`, `workflows/e2e-tests.yaml` |
| A6 | `DEVELOPMENT_PLAN.md` committed | — |
| C1 | Development and release tags are both created on `main`; development tags only after the `go-sdk` merge gate | ADR-010 |
| C2 | First version `v0.1.0`; the repositories are public | ADR-010 |
| C3 | End-to-end tests are separate Tester tasks; developers only need `make check` to pass | GL-003 |
| C4 | The frontend has a `Makefile` whose `check` target installs, lints, and builds | `guest-management-fe/Makefile` |
| C5 | `e2e-tests` stays a separate request | `workflows/e2e-tests.yaml` |
| C6 | Workspace subagents retired; `DEVELOPMENT_PLAN.md` is history up to 2026-09-17 | `Guest Management/CLAUDE.md`, `.claude/retired-agents/` |

## Open questions

### B. Product and operations (details in PRD-001, DB-001, UX-001)

| # | Question |
|---|----------|
| B1 | Permission gates and taking the tenant from the token for tenants, events, categories, workflow steps and their templates, and message templates. **This is a security gap** (PRD-001 Q4). |
| B2 | Load targets, release milestones, and success measurement (PRD-001 Q1, Q2). |
| B3 | `name` on users, `user_id` on `TokenPair`, `GET /roles`, and date and `contains` filters, all needed by the frontend (UX-001 Q1–Q3, Q8). |
| B4 | Environments, hosting, CI, secret custody, backups, and PII retention. These would feed a Deployment Plan, a Runbook, and DB-001 (PRD-001 Q9, Q11; DB-001 Q1, Q2). |
| B5 | Test strategy beyond GL-003: integration tests for transactions and frontend tests. This would feed a Test Plan. |
| B6 | Design decisions for B10, B13, and B14 (PRD-001 Q5, Q10). |
| B7 | Stale repository docs (`API_CONTRACT.md`, `TESTING.md`, parts of `DATABASE.md`, and the `serializer.ParseJSON` line in the AGENTS.md Definition of Done): fix them, or point them to these documents? |
