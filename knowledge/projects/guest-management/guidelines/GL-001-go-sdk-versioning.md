---
type: guideline
id: GL-001
title: "go-sdk versioning and consumption"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
depends_on: []
references: [ADR-010]
---

# go-sdk versioning and consumption

## Applies To

Developer, planner, and reviewer tasks that touch the `go-sdk` or `backend` repositories (ADR-010).

## Tools

| Tool | Purpose | Location |
|------|---------|----------|
| Go modules | Dependency on `github.com/biairmal/go-sdk` and `github.com/biairmal/go-sdk/mocks` | `backend/go.mod` |
| Git tags | Versions: release `vX.Y.Z`, development `vX.Y.Z-dev.N`, each also as `mocks/…` | `go-sdk` repository, `main` branch |

## Rules

1. The backend's `go.mod` must not contain `replace` directives for `go-sdk`. Use tagged versions only.
2. Put capabilities that are reusable across services in `go-sdk` (see `go-sdk/AGENTS.md`), not in the backend.
3. When a backend change needs a new SDK capability, the plan has a `go-sdk` task first and a backend task that depends on it. The backend task description names the SDK version to require (the next `-dev.N` tag). The owner creates that tag after merging the `go-sdk` task.
4. Agents never create, move, or push tags.
5. Keep `go-sdk/mocks/go.mod` requiring the `go-sdk` version it is released with.
6. A reviewer blocks any backend change that adds `replace` or requires an untagged pseudo-version of `go-sdk`.

## How to Run

The owner cuts a tag after a merge:

```sh
git tag v0.2.0-dev.1 && git tag mocks/v0.2.0-dev.1 && git push origin v0.2.0-dev.1 mocks/v0.2.0-dev.1
```

Then the backend is bumped with:

```sh
go get github.com/biairmal/go-sdk@v0.2.0-dev.1 github.com/biairmal/go-sdk/mocks@v0.2.0-dev.1 && go mod tidy
```

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **References:** [[ADR-010-consume-go-sdk-through-release-and-development-tags|ADR-010 · Consume go-sdk through tagged versions, with separate development and release tags]]
<!-- relations:end -->
