---
type: adr
id: ADR-004
title: "Use one role and permission engine with system and event scopes and an event-scoped fallback"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-09-13"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [API-001, DB-001]
references: []
---

# Use one role and permission engine with system and event scopes and an event-scoped fallback

<!-- Recorded from STAFFING_RBAC.md, FEATURES.md#roles, DEVELOPMENT_PLAN.md B6 and A9 (commits 2026-09-05, 2026-09-13). -->

## Context and Problem Statement

A user has a tenant-wide standing (Tenant Admin, Tenant Staff), but can also work one event in a narrow role (Usher, Photobooth Staff). An Usher must be able to scan tickets on their event without holding a tenant-wide role that grants scanning everywhere. How are roles and permissions modelled and checked?

## Decision Drivers

- REQUIREMENT.md §4.6 describes permission levels: guest management, scanning only, workflow management, and full event admin.
- One mechanism, extended by adding data rather than code.
- Least privilege for event staff.

## Considered Options

- One `roles` table with a `scope` column (`system` or `event`), shared permissions, and a check that tries the system role first and then falls back to the caller's event assignment on routes with `{event_id}`
- Separate engines for tenant roles and event permissions (a permission list on each assignment, as in the original REQUIREMENT.md §3.3)
- Permission codes stored directly on users

## Decision Outcome

**Chosen option:** "One roles table with a scope column, and an event-scoped fallback", because a single `PermissionResolver` serves both scopes and new roles or permissions are data only.

- The seeded catalogue (migration 000014) has the roles Super Admin, Tenant Admin, and Tenant Staff (system scope) and Usher and Photobooth Staff (event scope). The permissions are `manage_tenants`, `manage_users`, `manage_events`, `manage_staff`, `manage_guests`, `manage_workflows`, and `check_in`.
- `users.role_id` must reference a system-scope role, and `event_staff_assignments.role_id` must reference an event-scope role. Both rules are validated in the services.
- A partial unique index ensures there is exactly one Super Admin.
- `authz.RequirePermission` calls `RequireForEvent` when the route has `{event_id}`. It checks the system role first and, only if that fails with 401 or 403, checks the active assignment on that event. Resolver errors are never masked as 403.
- Each feature owns its permission codes. `internal/core/authz` provides only the mechanism.
- Role-to-permission lookups are cached in Redis (read-through), and a Redis failure falls back to the database.

## Consequences

- **Positive:** An Usher can scan only on assigned events. Adding a role is a migration.
- **Negative:** Changing roles requires a deploy. A user has only one system role. The seeded role ids are fixed UUIDs that other code depends on.
- **Follow-up:** Admin CRUD for roles, per-tenant roles, and multiple roles per user are on the roadmap (PRD-001 non-goals). The frontend hardcodes the role list because there is no `GET /roles`.

## Pros and Cons of the Options

### One engine with scopes and a fallback

- Good, because one resolver, one cache, and one set of tests serve both scopes.
- Bad, because an event route runs an extra lookup when the system check fails.

### Separate engines

- Good, because each engine is simple.
- Bad, because there would be two vocabularies and duplicated checks.

### Permissions on users

- Good, because it is maximally flexible.
- Bad, because there is no reuse and it can't be audited or managed.

## Confirmation

`authz__test.go` and `event_role_resolver__test.go` cover the A9 acceptance criteria. The seed migration is the single source of the catalogue.

## More Information

STAFFING_RBAC.md supersedes the role pseudo-code in REQUIREMENT.md §3.2 and §3.3.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]], [[DB-001-guest-management-database-design|DB-001 · Guest Management — Database Design]]
<!-- relations:end -->
