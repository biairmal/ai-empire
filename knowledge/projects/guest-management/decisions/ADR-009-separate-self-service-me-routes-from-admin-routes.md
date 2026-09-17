---
type: adr
id: ADR-009
title: "Separate self-service /me routes from admin routes"
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
affects: [API-001, UX-001]
references: []
---

# Separate self-service /me routes from admin routes

<!-- Recorded from DEVELOPMENT_PLAN.md B11 (route-split amendment) and PATTERNS.md "/me sub-resource pattern". -->

## Context and Problem Statement

B11 first shipped a single route, `POST /users/{id}/password`, which branched on "is the id the caller's own id?" to decide between a self-service change and an admin reset. The product owner rejected this. How should actions on one's own resources be separated from admin actions on someone else's?

## Decision Drivers

- An admin changing another user's password is a different action from a user changing their own.
- The frontend must never be trusted to supply "my own id".
- Authorisation should be decided by the route, not by branches inside services.

## Considered Options

- Separate routes: `/users/me/...` resolves the target from the JWT subject and needs only a valid token, while `/users/{id}/...` is admin-only and gated by permission middleware
- One route with a branch on `id == caller`
- A self-service route that takes the caller's id in the URL

## Decision Outcome

**Chosen option:** "Separate `/me` and `/{id}` routes", because the authorisation of each route is static and the self-service target can't be spoofed.

- `POST /api/v1/users/me/password` sets `must_change_password = false`.
- `POST /api/v1/users/{id}/password` requires `manage_users` and sets `must_change_password = true`.
- Both share an unexported helper for the actual update.
- chi matches the static `me` segment before `{id}`, whatever the registration order.
- This is the standing convention for every future action a user takes on their own resources (profile, and so on). Only the password route exists today.

## Consequences

- **Positive:** Clear authorisation, no id spoofing, and simpler services.
- **Negative:** Two routes and two handlers for similar logic.
- **Follow-up:** None. Add further `/me` routes only when a story needs them.

## Pros and Cons of the Options

### Separate routes

- Good, because security is decided statically per route.
- Bad, because there are more endpoints.

### One route with a branch

- Good, because there are fewer endpoints.
- Bad, because the authorisation lives inside the service and the frontend supplies the id.

### Self-service route with the caller's id in the URL

- Good, because routes are uniform.
- Bad, because the target is still chosen by the client.

## Confirmation

`user_service__test.go` covers both routes and the different values each sets for `must_change_password`. The UX (UX-001) keeps the two dialogs as separate components.

## More Information

None.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]], [[UX-001-login-and-user-management|UX-001 · Login and User Management — UX Specification]]
<!-- relations:end -->
