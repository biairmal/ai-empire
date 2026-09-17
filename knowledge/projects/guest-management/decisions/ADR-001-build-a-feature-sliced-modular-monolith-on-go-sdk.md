---
type: adr
id: ADR-001
title: "Build the backend as a feature-sliced modular monolith on go-sdk"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-07-04"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [ARCH-001]
references: []
---

# Build the backend as a feature-sliced modular monolith on go-sdk

<!-- Recorded after the fact from guest-management-be ARCHITECTURE.md and AGENTS.md (guidelines commit 2026-07-04). -->

## Context and Problem Statement

The backend has many domains (tenants, users, events, tickets, guests, scans, and more), but it is built by one person and deployed as a single service. The check-in path (`scans`) may later need to scale on its own. How should the code be organised so that it is simple to run today and features can be split out later?

## Decision Drivers

- One developer, many AI agents: rules must be simple to follow and to check.
- `scans` is write-heavy and a likely candidate to become a separate service.
- Cross-cutting infrastructure (HTTP, errors, logging, repositories, auth, crypto, queue) should be reused across the owner's services.
- Each layer should be testable against generated mocks.

## Considered Options

- Modular monolith with feature slices, built on a shared `go-sdk`
- Classic layered monolith (`handlers/`, `services/`, `repositories/`)
- Microservices from the start

## Decision Outcome

**Chosen option:** "Modular monolith with feature slices, built on a shared `go-sdk`", because it keeps one deployable service while making each slice a future service boundary, and it moves reusable infrastructure into a library shared by the owner's projects.

Rules that follow from it:

- The layers are handler → service → repository → go-sdk, and dependencies only point downward.
- `internal/features/<feature>` is a vertical slice, and features never import each other's internals. When cross-feature use is needed, it goes through `internal/core`, a small interface declared by the consumer, or an exported read repository.
- A feature with more than one entity is split into per-entity subpackages, never per-layer subpackages.
- `internal/app` is the only composition root.
- `errorz` codes are used in every layer and mapped to HTTP status codes only at the edge.

## Consequences

- **Positive:** Features stay independent: build order is `events` ← `tickets` ← `guests` ← `scans`. Services are tested with gomock. Adding a feature is a checklist (NEW_FEATURE_CHECKLIST.md).
- **Negative:** Cross-feature needs require more ceremony (for example the `TicketTypeSeeder` interface). The backend depends on the sibling `go-sdk` checkout through a relative `replace`.
- **Follow-up:** Decide how `go-sdk` is resolved outside the local workspace (the AI Empire worktrees and CI). See ARCH-001 risks.

## Pros and Cons of the Options

### Modular monolith with feature slices

- Good, because slices can be extracted later with changes only in wiring.
- Good, because a whole feature can be found in one folder.
- Bad, because shared logic can drift into copies unless `internal/core` is used. This already happened with list-parameter conversion and was fixed in A8.

### Classic layered monolith

- Good, because the layout is familiar.
- Bad, because a feature is spread across folders and the boundaries between features erode.

### Microservices from the start

- Good, because services are isolated and scale independently.
- Bad, because the operational cost is far too high for one developer before any release.

## Confirmation

Checked by review against the guest-management-be AGENTS.md "Definition of Done", with `make check` required to pass. `.golangci.yml` has no import-boundary rule (such as depguard), so the AI reviewer must check for cross-feature imports and handlers that touch the database. Adding a lint rule for this is an open improvement.

## More Information

Revisit when `scans` latency or load requires it to run as a separate service.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[ARCH-001-guest-management-architecture-overview|ARCH-001 · Guest Management — Architecture Overview]]
<!-- relations:end -->
