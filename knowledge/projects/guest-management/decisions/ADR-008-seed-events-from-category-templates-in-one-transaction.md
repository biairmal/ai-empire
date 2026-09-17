---
type: adr
id: ADR-008
title: "Seed new events from category templates by copying them in one transaction"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-09-15"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [API-001, DB-001]
references: []
---

# Seed new events from category templates by copying them in one transaction

<!-- Recorded from DEVELOPMENT_PLAN.md B7 and B12 Technical Designs and FEATURES.md#events (commit 2026-09-15). -->

## Context and Problem Statement

Categories carry default workflow steps and default ticket types. An event should start from those defaults and then be customised freely (REQUIREMENT.md §7). How are the defaults applied, and what happens when templates change later?

## Decision Drivers

- Changes at event level must not leak back into templates, and template changes must not rewrite past events.
- Event creation must not leave a half-seeded event.
- `events` must not import `tickets`, which would create an import cycle (see ADR-001).
- An organiser who forgets to configure a ticket type should not block guests at the door.

## Considered Options

- Copy the templates when the event is created, inside one transaction, with no link back; call the ticket seeding through a `TicketTypeSeeder` interface owned by `events` and wired in `internal/app`
- Link events to templates live and resolve them when read
- Copy best-effort, logging and skipping failures (the original B4 behaviour)

## Decision Outcome

**Chosen option:** "Copy at creation inside one transaction, with no link back", because it gives atomic creation and independent customisation, and it respects the direction of feature dependencies.

- `EventService.Create` runs the event insert, the workflow-step template copy, and the ticket-type template seed in one `sqlkit.DB.WithTransaction`. Any failure rolls everything back.
- A seeded or manually created ticket type applies to all of the event's current steps by default ("fail open"). This default applies best-effort outside a transaction when a ticket type is created manually.
- Templates have no foreign key from the event-level rows.

## Consequences

- **Positive:** Events are always fully seeded or not created at all. Organisers start from sensible defaults.
- **Negative:** Template improvements don't reach existing events. Creation takes longer when there are many templates. A new ticket type grants all steps until someone narrows it.
- **Follow-up:** Workflow-step sync and ticket-type step replacement are still not transactional (see PRD-001 risks).

## Pros and Cons of the Options

### Copy in one transaction

- Good, because it is atomic and events are independent of templates.
- Bad, because there is no propagation of template changes.

### Live link

- Good, because template changes propagate.
- Bad, because customisation needs override tracking and a template edit silently changes live events.

### Best-effort copy

- Good, because event creation never fails because of templates.
- Bad, because an event can be saved with only some of its defaults, silently.

## Confirmation

`event_service__test.go` unit-tests the transaction body (`createEvent`, `copyWorkflowStepTemplates`) and error propagation with a stub seeder. The rollback itself is not tested: the backend has no `*_integration_test.go` against a live database, which is a test gap.

## More Information

This was the first use of transactions in the codebase, after `users.TransferMaster`.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]], [[DB-001-guest-management-database-design|DB-001 · Guest Management — Database Design]]
<!-- relations:end -->
