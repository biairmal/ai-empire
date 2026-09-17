---
type: adr
id: ADR-002
title: "Enforce tenant isolation in the application layer using the token's tenant claim"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-09-05"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [ARCH-001, API-001, DB-001]
references: []
---

# Enforce tenant isolation in the application layer using the token's tenant claim

<!-- Recorded from STAFFING_RBAC.md §6, FEATURES.md (staffing, users) and DEVELOPMENT_PLAN.md B6/B11. -->

## Context and Problem Statement

All tenants share one PostgreSQL database, and almost every table carries `tenant_id`. The earliest slices (`tenants`, `users`, `events`) accepted `tenant_id` in the request body, so a caller could name another tenant's data. How is isolation enforced, and how should cross-tenant access look to the caller?

## Decision Drivers

- REQUIREMENT.md §5.1: no guests, events, or staff are shared between tenants.
- Permission checks mean nothing if the caller can choose the tenant.
- Callers must not be able to tell whether a resource exists in another tenant.

## Considered Options

- Shared schema; tenant taken from the JWT `tenant_id` claim and checked in services; cross-tenant access returns 404
- Shared schema; tenant taken from the request body
- PostgreSQL row-level security, or a schema or database per tenant

## Decision Outcome

**Chosen option:** "Shared schema; tenant taken from the JWT claim and checked in services; cross-tenant access returns 404", because it closes the body-spoofing hole with no infrastructure change and matches the existing repository pattern.

- The services resolve the tenant with `authz.TenantIDFromContext`. Request DTOs carry no `tenant_id`.
- Nested resources are always resolved within the parent in the URL (`{event_id}`, `{category_id}`). A child of a different parent returns 404.
- A resource in another tenant returns 404, never 403 ("not found, not forbidden").

## Consequences

- **Positive:** `staffing` (B6) and `users` (B11) are isolated. The 404 convention prevents probing for resources.
- **Negative:** Isolation depends on every service remembering the check, and the database does not enforce it.
- **Follow-up:** The rollout is incomplete. `events` still takes `tenant_id` from the body, and `tenants`, `event-categories`, `events`, `workflow-steps`, `workflow-step-templates`, and `message-templates` only require a login. Tracked as PRD-001 Q4.

## Pros and Cons of the Options

### Tenant from the JWT, checked in services

- Good, because it needs no database change and is easy to unit-test.
- Bad, because a forgotten check is a data leak.

### Tenant from the request body

- Good, because it is simple.
- Bad, because any authenticated user can address any tenant.

### Row-level security, or a schema or database per tenant

- Good, because the database enforces isolation.
- Bad, because it needs per-request session variables or connection routing, which the go-sdk repository does not support today.

## Confirmation

Table-driven service tests cover cross-tenant cases that return 404. The AI reviewer rejects new DTOs that accept `tenant_id`.

## More Information

Revisit row-level security if a tenant requires contractual isolation, or after any isolation incident.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[ARCH-001-guest-management-architecture-overview|ARCH-001 · Guest Management — Architecture Overview]], [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]], [[DB-001-guest-management-database-design|DB-001 · Guest Management — Database Design]]
<!-- relations:end -->
