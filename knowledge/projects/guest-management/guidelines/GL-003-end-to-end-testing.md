---
type: guideline
id: GL-003
title: "End-to-end testing with Postman and newman"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Product Owner"
created: "2026-09-17"
updated: "2026-09-17"
depends_on: []
references: [API-001]
---

# End-to-end testing with Postman and newman

## Applies To

Tester tasks (the `e2e-tests` workflow) in the `backend` repository. Unit tests remain the developer's responsibility (`backend/AGENTS.md`, `docs/TESTING.md`).

## Tools

| Tool | Purpose | Location |
|------|---------|----------|
| Postman collection (v2.1) | One collection, one folder per feature, one request per scenario, with test scripts as assertions | `backend/postman/guest-management.postman_collection.json` |
| Postman environment | Base URL and test accounts as variables; no real secrets committed | `backend/postman/local.postman_environment.json` |
| newman (run with `npx`) | Runs the collection from the command line | `make test-e2e` |
| API contract | The request and response shapes to test against | API-001 and `backend/api/swagger` |

## Rules

1. Every acceptance criterion in scope has at least one request with an assertion. Also cover the documented errors in API-001: 400, 401, 403, 404 (including cross-tenant and cross-event ids), 409, and 422.
2. Assert the response envelope (`code`, `data` / `error.code`), not only the status code.
3. Each scenario creates its own data (tenant, users, event, …) through the API, or through documented seed steps, and doesn't depend on state left by another run.
4. Log in through `POST /api/v1/auth/login` and store tokens in collection variables. Test both a permitted and a forbidden role for permission-gated endpoints.
5. Hand-built query strings must percent-encode `;` as `%3B` (for example `name=Ali%3Blike`).
6. `postman/README.md` explains prerequisites (running API, migrated database, seed data) and the single `make test-e2e` command.
7. Don't change production code. Report defects in the summary.

## How to Run

With the API running (`docker compose up -d`, `make migration-up`, `make run`):

```sh
make test-e2e
```

End-to-end tests are always separate Tester tasks (`e2e-tests` workflow). They are not part of any merge gate: developer tasks only need `make check` to pass, and `make check` never runs newman (owner decision, 2026-09-17).

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **References:** [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]]
<!-- relations:end -->
