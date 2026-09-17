---
type: adr
id: ADR-003
title: "Use stateless HS256 access and refresh JWTs without server-side revocation"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-08-06"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [API-001]
references: []
---

# Use stateless HS256 access and refresh JWTs without server-side revocation

<!-- Recorded from FEATURES.md#auth and DEVELOPMENT_PLAN.md B3 (auth commit 2026-08-06). -->

## Context and Problem Statement

Staff users must log in, and every protected route needs to know the caller, their tenant, and their role. The `users` feature may later become a separate identity service. What token scheme should be used?

## Decision Drivers

- The design is monolith-first, but switching to remote validation or JWKS later must need only a configuration change (go-sdk `auth`).
- The service should not keep session state.
- A role downgrade must take effect promptly (STAFFING_RBAC.md §6).

## Considered Options

- Stateless access and refresh JWTs signed with HS256 by go-sdk `auth`
- Server-side sessions in Redis
- An external identity provider (OIDC)

## Decision Outcome

**Chosen option:** "Stateless access and refresh JWTs signed with HS256", because it reuses go-sdk `auth` with no extra storage, and RS256 or JWKS can be enabled later by configuration.

- Access tokens last 15 minutes and refresh tokens 7 days (`AUTH_ACCESS_TTL`, `AUTH_REFRESH_TTL`).
- The claims are `sub` (the user id), `type` (`access` or `refresh`), `tenant_id`, and `role_id`. Protected routes reject refresh tokens (`AccessOnlyValidator`).
- A refresh reloads the user, so a deleted user can't refresh and `role_id` is always current.
- Login returns the same 401 for an unknown email and a wrong password.
- Routes are protected by default (`default_protected: true`). Public routes are listed in `configs/config.yaml`.

## Consequences

- **Positive:** No session store is needed, the service scales horizontally, and the identity service can be split out later.
- **Negative:** There is no logout or revocation. A stolen token is valid until it expires, and a role change waits for the next refresh (up to 15 minutes).
- **Follow-up:** Decide whether revocation or refresh-token rotation tracking is needed before production (PRD-001 risks).

## Pros and Cons of the Options

### Stateless JWTs

- Good, because they need no storage and are already provided by go-sdk.
- Bad, because they can't be revoked before expiry.

### Redis sessions

- Good, because sessions can be revoked immediately.
- Bad, because every request makes a stateful lookup, and moving to an identity service is harder.

### External identity provider

- Good, because it offers mature features (MFA, password reset e-mail).
- Bad, because it adds cost and an external dependency before the first release.

## Confirmation

`auth_service__test.go` covers token-type rejection and reloading the user on refresh. The route policy in `configs/config.yaml` is reviewed for any new public route.

## More Information

Revisit before a production launch, or if a security review requires revocation.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]]
<!-- relations:end -->
