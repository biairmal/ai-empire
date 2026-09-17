---
type: adr
id: ADR-010
title: "Consume go-sdk through tagged versions, with separate development and release tags"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-09-17"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [ARCH-001]
references: [ADR-001]
---

# Consume go-sdk through tagged versions, with separate development and release tags

## Context and Problem Statement

`guest-management-be` consumes `go-sdk` (and its nested `mocks` module) through `replace => ../go-sdk`. This only works when both repositories are checked out side by side. AI Empire builds each task in its own worktree (`<workspaces>/guest-management/<repo>/task-N`), and CI would build from a fresh clone, so `../go-sdk` doesn't exist there and backend builds fail. How should the backend depend on `go-sdk`?

## Decision Drivers

- Backend builds must be reproducible from a clean clone.
- SDK changes must be usable by the backend before a stable SDK release exists.
- Stable releases must stay clearly distinguishable from work in progress.
- No extra infrastructure (no private module proxy).

## Considered Options

- Tagged module versions: development tags (semantic-versioning pre-releases) and release tags, both on `main`
- A `go.work` file with a checkout of `go-sdk` next to every worktree
- A platform hook that clones `go-sdk` next to each worktree, keeping `replace`
- Pseudo-versions of untagged commits

## Decision Outcome

**Chosen option:** "Tagged module versions with separate development and release tags", decided by the owner on 2026-09-17.

- **Release tags:** `vMAJOR.MINOR.PATCH` (for example `v0.2.0`), created only on commits on `go-sdk`'s `main` branch.
- **Development tags:** semantic-versioning pre-releases of the next version, `vMAJOR.MINOR.PATCH-dev.N` (for example `v0.2.0-dev.1`), for SDK changes the backend needs before a release. These are also created on `main`, only after the `go-sdk` change has passed its merge gate, so a dependent backend task always waits for an approved SDK change (owner confirmation, 2026-09-17).
- **Nested module:** `mocks` is its own Go module, so every tag is created twice: `vX.Y.Z[-dev.N]` and `mocks/vX.Y.Z[-dev.N]`. At each tag, `mocks/go.mod` must require the matching `go-sdk` version rather than a placeholder pseudo-version.
- **Backend:** `go.mod` requires real tags for `github.com/biairmal/go-sdk` and `github.com/biairmal/go-sdk/mocks`, and contains no `replace` directives on `main`. A local `go.work` (git-ignored) may point to a sibling checkout for side-by-side development.
- **Tags are created by the owner**, after the `go-sdk` merge gate. AI agents never tag or push. A backend task that needs an unreleased SDK change depends on the `go-sdk` task and runs after the tag exists.

## Consequences

- **Positive:** Backend builds work in AI Empire worktrees and in CI. Every backend commit records exactly which SDK version it uses.
- **Negative:** Every SDK change the backend needs costs one tag and one `go get` bump. A backend task planned right after a `go-sdk` task can't build until the owner has tagged it.
- **Follow-up (done 2026-09-17):** the repositories are public, so no `GOPRIVATE` or credentials are needed. `mocks/go.mod` requires `go-sdk v0.1.0`; `go-sdk` is tagged `v0.1.0` and `mocks/v0.1.0`; the backend's `replace` lines are removed, it requires both `v0.1.0` tags, and `go.work` is git-ignored.

## Pros and Cons of the Options

### Tagged versions with development and release tags

- Good, because builds are reproducible and Go-native, and stable versions are clearly separate from work in progress.
- Bad, because of the extra tagging step and the cross-repository ordering.

### `go.work` with a sibling checkout

- Good, because there is no tagging.
- Bad, because the build depends on whatever SDK commit happens to be checked out, and the platform must manage that checkout.

### Clone hook with `replace` kept

- Good, because nothing in the backend changes.
- Bad, because it needs a platform change and builds aren't reproducible.

### Pseudo-versions

- Good, because they need no tags.
- Bad, because they are unreadable and make it unclear what is a release.

## Confirmation

`go mod verify` and `grep replace go.mod` (expected to find nothing) in backend review. The `deps-verify` step of `make check` resolves modules without a sibling checkout.

## More Information

Revisit if `go-sdk` gets a second consumer, or if its release cadence needs automated tagging.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[ARCH-001-guest-management-architecture-overview|ARCH-001 · Guest Management — Architecture Overview]]
- **References:** [[ADR-001-build-a-feature-sliced-modular-monolith-on-go-sdk|ADR-001 · Build the backend as a feature-sliced modular monolith on go-sdk]]
<!-- relations:end -->
