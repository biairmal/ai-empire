---
type: adr
id: ADR-007
title: "Soft-delete domain entities and keep scan logs append-only"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-07-05"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [DB-001]
references: []
---

# Soft-delete domain entities and keep scan logs append-only

<!-- Recorded from DATABASE.md §1, §5, ARCHITECTURE.md and FEATURES.md#scans. -->

## Context and Problem Statement

Events, guests, tickets, and staff assignments are business records that may be referenced later (history, reports, disputes). Scans are evidence of what happened at the venue. How are deletes and audit fields handled?

## Decision Drivers

- Staffing history must record everyone who was ever assigned (STAFFING_RBAC.md §5).
- Scan counts must be permanent and accurate (B9).
- One mechanism for every repository.

## Considered Options

- Soft delete (`deleted_at`) through a shared audit repository decorator, with `scan_logs`, the junction tables, and the reference tables kept without soft delete
- Hard delete everywhere
- Hard delete plus separate history tables

## Decision Outcome

**Chosen option:** "Soft delete through a shared audit decorator", because it keeps history with no extra tables, and the decorator applies `deleted_at IS NULL` and the timestamps in one place.

- Tables with soft delete: tenants, users, event_categories, workflow_step_templates, events, workflow_steps, event_staff_assignments, ticket_types, ticket_type_templates, guests, tickets, message_templates.
- Tables without it (built with `NewRepositoryNoAudit`): roles, permissions, role_permissions, scan_logs, ticket_type_workflow_steps.
- Uniqueness that must allow re-creation after a delete uses partial unique indexes (for example, active staff assignments).
- `scan_logs` are never updated or deleted, and their repository has no cache, so the duplicate check always sees the latest writes.

## Consequences

- **Positive:** History is preserved and queries are consistent.
- **Negative:** Plain `UNIQUE` constraints block re-creating a deleted record unless they are made partial (fixed for staffing in migration 000015). Deleted personal data stays in the database.
- **Follow-up:** Define purging or anonymisation of soft-deleted guest data (PRD-001 Q11). Check the remaining plain unique constraints (for example `ticket_types(event_id, name)`) for the same re-creation problem.

## Pros and Cons of the Options

### Soft delete through a decorator

- Good, because history is kept and the logic lives in one place.
- Bad, because it interacts with unique constraints, and data keeps growing.

### Hard delete

- Good, because it is simple and keeps the data small.
- Bad, because history and evidence are lost.

### Hard delete plus history tables

- Good, because live tables stay clean and history is complete.
- Bad, because it doubles the schema and needs triggers or code for every table.

## Confirmation

Repositories are built only through `internal/core/repository.NewRepository` or `NewRepositoryNoAudit`. The AI reviewer checks that new tables follow the list above.

## More Information

None.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[DB-001-guest-management-database-design|DB-001 · Guest Management — Database Design]]
<!-- relations:end -->
