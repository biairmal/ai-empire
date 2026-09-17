---
type: prd
id: PRD-001
title: "Guest Management Platform"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Product Owner"
created: "2026-09-17"
updated: "2026-09-17"
derived_from: []
depends_on: []
references: []
---

# Guest Management Platform

<!--
Extracted on 2026-09-17 from the existing Guest Management workspace (C:\Dev\Projects\Guest Management):
guest-management-be/docs/REQUIREMENT.md, STAFFING_RBAC.md, FEATURES.md, DEVELOPMENT_PLAN.md (stories B1–B14, Track C).
This is the product-level PRD. New feature requests should reference it (depends_on: [PRD-001]).
-->

## Summary

Guest Management is a multi-tenant platform for organisations that run events: they manage guest lists, send invitations and collect RSVPs, issue QR tickets, and check guests in through configurable workflow steps (check-in, souvenir pickup, photo booth, VIP lounge, checkout) on the event day. Each tenant can reuse app-wide defaults, override them at tenant level, and customise them for a single event. Staff access is role-based at two levels: tenant-wide and per event. The first release gives an organiser one system for the whole guest journey, from invitation to post-event thank-you, instead of spreadsheets, messaging apps, and manual door lists.

## Background and Problem Statement

**Current situation:** The backend (`guest-management-be`) covers tenants, users, authentication, events and categories, workflow steps and templates, message templates, event staffing with roles and permissions, ticket types and templates, guests with RSVP and QR tickets, and ticket scanning. The frontend (`guest-management-fe`) has login and user-management screens running on mock data only. Post-event messaging, event reports, and incidents have written stories but no design or implementation yet.

**Problem:** Organisers of large and multi-day events need to control who is invited, who confirmed, what each guest is entitled to (for example VIP versus Regular), and what each guest has already done at the venue. Doing this by hand leads to duplicate souvenir pickups, VIP areas that are not enforced, no live view of attendance, and staff with more access than they need.

**Evidence:** The problem statement and requirements come from the product owner's specification (REQUIREMENT.md, STAFFING_RBAC.md) and the story acceptance criteria in DEVELOPMENT_PLAN.md. No usage data exists yet because the product has not been released.

## Goals and Success Metrics

No baselines or targets have been defined yet. The metrics below are proposed and are tracked as open questions Q1 and Q2.

| # | Goal | Metric | Baseline | Target | Measured by |
|---|------|--------|----------|--------|-------------|
| G1 | Admit guests only to the steps their ticket allows | Share of scans against a step the ticket type is not entitled to that are rejected | Not measured (not released) | 100% (enforced by the system) | `scan_logs` and API error logs |
| G2 | Prevent duplicate use of one-time steps | Duplicate completions of a step with `allows_multiple = false` | Not measured | 0 | `scan_logs` count per ticket and step |
| G3 | Fast check-in at the door | Scan request latency at peak | Not measured | To be defined (Q1) | Prometheus metrics (`guest_management` namespace) |
| G4 | Organisers run an event without spreadsheets | Events run end-to-end on the platform | 0 | To be defined (Q2) | Product analytics (not yet defined) |

## Non-Goals

These are out of scope for the first release. They come from DEVELOPMENT_PLAN.md Track C and the non-goals in each story.

- Tenant management and subscription/billing for monetising tenants.
- Admin CRUD for roles and permissions, per-tenant custom roles, and more than one role per user.
- Guest self-registration through a public form.
- A dedicated mobile or operations app for event-day staff. The API is the staff interface for now.
- Staff chat and real-time push notifications. Incident "alerting" means visibility through a list.
- Custom per-event guest fields. A guest has only name, email, and phone.
- Real email, SMS, or WhatsApp delivery in the first backend phases. Invitations are published to a queue, and no consumer sends them yet.
- A per-guest "current step" drill-down in reports. Reports show aggregate counts only.

## Users and Personas

| Persona | Description | Primary needs | Frequency of use |
|---------|-------------|---------------|------------------|
| Super Admin | The single platform-level operator, who belongs to a reserved System tenant | Manage tenants and everything within them | Occasional |
| Tenant Master | At most one user per tenant who owns the tenant account. This is a flag on a user, not a role | Administer users and hand the ownership over to someone else | Weekly |
| Tenant Admin | Full administrator within one tenant | Manage users, events, staff, guests, and workflows | Daily around events |
| Tenant Staff | Baseline operational staff member | Manage guests and check guests in | Daily around events |
| Event staff (Usher, Photobooth Staff) | A tenant user assigned to one event with an event-scoped role | Scan tickets for that event only | During events |
| Guest | An invited person with no account | Receive an invitation, confirm or decline, and present a QR ticket | Per event |

## User Stories

### US-01: Configure an event from templates

As a **Tenant Admin**, I want **new events to start with the workflow steps and ticket types defined for their category** so that **I don't have to rebuild the same setup for every event**.

**Acceptance criteria**

- Given a category with workflow-step templates and ticket-type templates, when I create an event in that category, then the event has a matching workflow step and ticket type for each template.
- Given a category without templates, when I create an event, then the event has no steps or ticket types.
- Given a template is changed after an event was created, when I view that event, then its steps and ticket types are unchanged.
- Given seeding fails partway through, when I create the event, then nothing is saved.

### US-02: Define ticket types and their entitlements

As a **Tenant Admin**, I want **to define ticket types (for example VIP and Regular) and choose which workflow steps each one includes** so that **check-in enforces what each guest is entitled to**.

**Acceptance criteria**

- Given an event, when I create a ticket type whose name already exists on that event, then it is rejected with 409.
- Given a new ticket type, when it is created, then it applies to all of the event's current workflow steps by default.
- Given a workflow step from another event, when I assign it to a ticket type, then the request is rejected.

### US-03: Invite guests and collect RSVPs

As a **Tenant Staff member**, I want **to add guests, assign each one a ticket type, and send an invitation** so that **guests can confirm or decline without logging in**.

**Acceptance criteria**

- Given a guest without a ticket type, when I send an invitation, then it is rejected with 400.
- Given a guest with a ticket type, when I send an invitation, then the guest's RSVP status becomes `invited` and an unguessable invitation token is generated.
- Given an invitation token, when the guest confirms through the public RSVP endpoint, then the status becomes `confirmed`. If the event requires RSVP, a QR ticket is also issued.
- Given an event that does not require RSVP, when the invitation is sent, then the ticket is issued immediately. A later decline does not void it.
- Given an unknown token, when it is used, then the response is 404.

### US-04: Check guests in at workflow steps

As an **Usher**, I want **to scan a guest's QR code at a specific workflow step** so that **one-time steps can't be claimed twice and repeatable steps are counted accurately**.

**Acceptance criteria**

- Given a ticket entitled to a step that it hasn't completed, when I scan it, then a scan log is recorded and an `active` ticket becomes `used`.
- Given a completed step with `allows_multiple = false`, when I scan the ticket again, then it is rejected with 409 and nothing is logged.
- Given a step with `allows_multiple = true`, when I scan the ticket repeatedly, then every scan is logged.
- Given a ticket from another event, or an `invalidated` ticket, or a step the ticket type doesn't include, when I scan it, then it is rejected.
- Given I only hold an event-scoped role on event E, when I scan on event E, then it succeeds, and when I scan on event F, then I get 403.

### US-05: Staff events with scoped roles

As a **Tenant Admin**, I want **to assign tenant users to an event with an event-scoped role** so that **staff get only the access they need for that event**.

**Acceptance criteria**

- Given a system-scoped role, when I assign it to an event, then the request is rejected with 400.
- Given a user who is already assigned to the event, when I assign them again, then the request is rejected with 409. After removing them, re-assigning creates a new assignment.
- Given an event or user from another tenant, when I act on it, then the response is 404, not 403.

### US-06: Administer tenant users

As a **Tenant Master**, I want **to manage my tenant's users, reset their passwords, and transfer my master status** so that **user administration is secure and not tied to one person**.

**Acceptance criteria**

- Given a caller without `manage_users`, when they call any user-administration endpoint, then the response is 403.
- Given a new user, when they log in, then the login succeeds and the response reports `must_change_password: true`.
- Given a user changes their own password, when the change succeeds, then `must_change_password` becomes false. Given an admin resets a user's password, then it becomes true.
- Given the caller is the current master, when they transfer master status to an active user in the same tenant, then exactly one master exists afterwards. A target in another tenant gets 404.

### US-07: Thank admitted guests after the event (not started)

As a **Tenant Staff member**, I want **admitted guests to receive a thank-you message with documentation links after the event** so that **the event closes properly without manual messaging**.

**Acceptance criteria**

- Given the event's end date has not passed, when I trigger post-event messages, then the request is rejected with 400.
- Given I trigger it twice, when the second run completes, then no guest is thanked twice and the response reports how many guests were newly notified and how many were already notified.

### US-08: Live event report (not started)

As an **organiser**, I want **live counts of guests by RSVP status and of distinct tickets that completed each step** so that **I can monitor the event at a glance**.

**Acceptance criteria**

- Given a scan or an RSVP was just recorded, when I request the report, then the counts include it.
- Given an event in another tenant, when I request its report, then the response is 404.

### US-09: Report incidents (not started)

As an **event staff member**, I want **to raise an incident for my event and see the incidents others raised** so that **issues are visible to everyone working the event**.

**Acceptance criteria**

- Given I create an incident, when it is saved, then its status is `open`. Its status can later move to `in_progress` and then `resolved`.
- Given an incident id from another event, when I request it, then the response is 404.

## Functional Requirements

| ID | Requirement | Priority | User story | Acceptance criteria |
|----|-------------|----------|------------|---------------------|
| FR-01 | The system shall manage tenants with a name, type, settings, and branding. | Must | — | CRUD at `/api/v1/tenants` works; deletes are soft |
| FR-02 | The system shall authenticate users by email and password and issue access and refresh tokens. Unknown email and wrong password return the same error. | Must | US-06 | Login returns a token pair or a generic 401 |
| FR-03 | The system shall support three configuration layers, app default, tenant override, and event customisation, for categories, workflow steps, ticket types, and message templates. | Must | US-01 | `source` rules are enforced (app ⇒ no tenant, tenant ⇒ tenant id) |
| FR-04 | The system shall copy a category's workflow-step templates and ticket-type templates onto a new event in one transaction. | Must | US-01 | US-01 criteria |
| FR-05 | The system shall let an event's workflow steps be added, renamed, reordered, and removed in a single sync call. | Must | US-01 | `PUT /events/{id}/workflow-steps` reconciles the whole list |
| FR-06 | The system shall manage ticket types per event and the set of workflow steps each one includes. | Must | US-02 | US-02 criteria |
| FR-07 | The system shall manage guests per event and assign each guest a ticket type from the same event. | Must | US-03 | A cross-event ticket type is rejected with 400 |
| FR-08 | The system shall send invitations with an unguessable token and publish an invitation message. | Must | US-03 | US-03 criteria |
| FR-09 | The system shall expose a public RSVP endpoint keyed by the invitation token and issue QR tickets according to the event's `rsvp_required` setting. | Must | US-03 | US-03 criteria |
| FR-10 | The system shall record scans by QR code and workflow step, enforce entitlement and single use, and keep an append-only scan log. | Must | US-04 | US-04 criteria |
| FR-11 | The system shall provide a shared role and permission engine with `system` and `event` scopes and a seeded catalogue. | Must | US-05 | Five roles and seven permissions are seeded, and only one Super Admin can exist |
| FR-12 | The system shall accept an event-scoped assignment as a fallback when a system role lacks the permission on routes that include `{event_id}`. | Must | US-04 | US-04 criterion on events E and F |
| FR-13 | The system shall assign tenant users to events with event-scoped roles, keeping the assignment history. | Must | US-05 | US-05 criteria |
| FR-14 | The system shall gate user administration on `manage_users`, take the tenant from the token, and support admin password reset, self-service password change, and tenant-master transfer. | Must | US-06 | US-06 criteria |
| FR-15 | The system shall manage email and WhatsApp message templates (email requires a subject, WhatsApp must not have one) at app, tenant, and event scope. | Must | — | Scope and channel rules are enforced with 400 |
| FR-16 | The system shall send thank-you messages with documentation links to admitted guests after an event, without duplicates. | Should | US-07 | US-07 criteria |
| FR-17 | The system shall provide a live, read-only event report with counts by RSVP status and per workflow step. | Should | US-08 | US-08 criteria |
| FR-18 | The system shall let event staff create, list, and update event incidents with a severity and a status. | Should | US-09 | US-09 criteria |
| FR-19 | The system shall provide a web UI for login, a forced password change, and user management (list, create, detail, reset, transfer, delete). | Must | US-06 | Covered by UX-001 |
| FR-20 | The system shall provide web UIs for events, guests, staff, templates, workflow steps, incidents, and a dashboard. | Should | US-01–US-09 | These appear as navigation items only today; no screens are designed yet (Q7) |

## Non-Functional Requirements

| ID | Category | Requirement | Target / threshold |
|----|----------|-------------|--------------------|
| NFR-01 | Security (isolation) | No guests, events, or staff are shared between tenants. A cross-tenant access attempt looks like a missing resource. | 404 for any cross-tenant or cross-event id on every tenant-scoped endpoint (see Q4 for gaps) |
| NFR-02 | Security (privacy) | Guest email and phone are encrypted at rest. The name stays searchable in plaintext. | AES-256-GCM plus an HMAC-SHA256 blind index; no plaintext email or phone in the database |
| NFR-03 | Security (access) | Password authentication and role-based authorisation. A role change takes effect at the next token refresh. | Access token 15 min, refresh token 7 days (configurable) |
| NFR-04 | Scalability | Support large events, high scanning volume, and bulk messaging. | Numbers not defined (Q1) |
| NFR-05 | Availability | Real readiness checks and graceful shutdown | `/ready` returns 503 when the database is down or during shutdown |
| NFR-06 | Observability | Structured logs with request and correlation ids, Prometheus metrics, optional OpenTelemetry traces | Enabled through configuration |
| NFR-07 | Abuse protection | Rate limiting per IP | Default 10 requests/s with a burst of 20 (in memory or Redis) |
| NFR-08 | Maintainability | Feature-sliced code; `make check` (format, lint, unit tests, coverage, vulnerability check) must pass | Required for every change (guest-management-be AGENTS.md) |
| NFR-09 | Accessibility | Web UI accessibility target | Not defined (Q8); WCAG 2.2 AA is proposed |

## Assumptions and Constraints

**Assumptions**

- Email is unique across all tenants, so login needs only an email. *To be verified by the product owner if tenants may share users in the future.*
- A guest is "admitted" (and receives a thank-you message) if they hold an issued ticket, not only if they were scanned. *To be decided in the B10 design (Q5).*
- Invitation delivery is asynchronous and best-effort. A failed publish does not fail the invitation call. *Verified in code.*

**Constraints**

- The backend is Go 1.25, uses chi, PostgreSQL, and Redis, and builds on the owner's `go-sdk` library. New third-party dependencies are allowed only when `go-sdk` lacks the capability.
- The frontend is Next.js 16, React 19, Ant Design 6, and Tailwind 4, managed with bun.
- Roles and permissions change only through database migrations. There is no admin UI for them.
- Access tokens are stateless, so there is no logout or revocation before a token expires.

## Dependencies

| Dependency | Owner | Needed by | Status |
|------------|-------|-----------|--------|
| `go-sdk` (httpkit, repository, sqlkit, auth, crypto, queue, lifecycle, metrics, tracer, ratelimit) | Bandana Irmal A | All backend work | Available; consumed through a local `replace => ../go-sdk` |
| Message consumer and sending provider (email, WhatsApp) | Not assigned | Real invitations, FR-16 | Not chosen (Q6) |
| Kafka | Infrastructure | Queue backend in production | Runs in local docker-compose only; the default backend is `noop` |
| `GET /roles` endpoint | Backend | Frontend role picker | Missing; the frontend hardcodes the role list |
| `name` field on users | Backend | Frontend user list | Missing (Q3) |

## Risks

| Risk | Likelihood | Impact | Mitigation | Owner |
|------|------------|--------|------------|-------|
| Endpoints for tenants, events, categories, workflow steps and their templates, and message templates only require a login, and events still take `tenant_id` from the request body, so a user from one tenant could read or change another tenant's data | High | High | Gate these endpoints on permissions and take the tenant from the token (Q4) | Bandana Irmal A |
| Two concurrent scans of a one-time step could both pass the duplicate check | Low | Medium | Wrap the scan in a transaction or add a partial unique index | Backend |
| Workflow-step sync and ticket-type step replacement are not transactional, so a failure can leave partial state | Low | Medium | Retry the call; add a transaction if failures are reported | Backend |
| A compromised token stays valid until it expires | Medium | Medium | Short access-token lifetime; revocation is a future decision | Bandana Irmal A |
| Loss of the PII encryption keys makes guest email and phone unrecoverable | Low | High | Key custody and backup procedure (Q9) | Operations |
| The repository documentation is drifting from the code (API_CONTRACT.md and TESTING.md are stale) | High | Low | Treat the documents in this knowledge folder and the Swagger spec as the source of truth | Bandana Irmal A |

## Release Scope

No release dates are set (Q2).

| Milestone | Scope (requirements) | Target date |
|-----------|----------------------|-------------|
| Backend foundations and domain, done (A1–A9, B1–B9, B11, B12) | FR-01–FR-15, NFR-01–NFR-08 | Shipped to `main` by 2026-09-15 |
| Frontend: login and user management | FR-19 (UI done with mock data; API integration pending) | Not set |
| Backend: post-event messages (B10), event reports (B13), incidents (B14) | FR-16–FR-18 (stories written, not designed) | Not set |
| Frontend: remaining screens | FR-20 | Not set |

## Open Questions

| # | Question | Owner | Due | Blocking? |
|---|----------|-------|-----|-----------|
| Q1 | What are the target load figures: guests per event, scans per minute at peak, and acceptable scan latency? | Bandana Irmal A | Not set | No |
| Q2 | What are the release milestones and dates, and how will success (G3, G4) be measured? | Bandana Irmal A | Not set | No |
| Q3 | Add a `name` field to users (migration, DTO, API)? The UI design depends on it. | Bandana Irmal A | Before the user-list API integration | Yes, for FR-19 |
| Q4 | Which permissions should gate tenants (`manage_tenants`?), events and categories (`manage_events`?), workflow steps (`manage_workflows`?), and message templates, and when does `events` switch to taking the tenant from the token? | Bandana Irmal A | Before any production use | Yes |
| Q5 | For B10, does "admitted" mean holding an issued ticket or having at least one scan? Is sending automatic (a scheduler) or triggered manually? Where are documentation links stored? How is "already thanked" tracked? | Bandana Irmal A | Before the B10 design | Yes, for FR-16 |
| Q6 | Which email and WhatsApp provider will be used, and who builds the queue consumer? | Bandana Irmal A | Before real invitations | No |
| Q7 | Which screens come after user management, in what order, and does the dashboard depend on B13? | Bandana Irmal A | Not set | No |
| Q8 | What is the accessibility target for the web UI (WCAG 2.2 AA?) and which browsers and devices must be supported (for example, phones at the door)? | Bandana Irmal A | Not set | No |
| Q9 | Which environments exist (staging, production), where are they hosted, and who holds and backs up the secrets (JWT secret, PII keys)? | Bandana Irmal A | Before the first deployment | No |
| Q10 | For B13 and B14, which permission gates reports and incidents (new codes or existing ones)? | Bandana Irmal A | Before those designs | No |
| Q11 | Is a guest's personal data retained after an event, and for how long (legal basis, deletion)? | Bandana Irmal A | Before production | No |

## Glossary

| Term | Definition |
|------|------------|
| Tenant | A customer organisation. Almost all data belongs to exactly one tenant |
| Tenant master | The single owner user of a tenant (`is_tenant_master`) |
| Workflow step | A stage a guest passes through at an event (check-in, photo booth, …); `allows_multiple` makes a step repeatable |
| Ticket type | A class of ticket (VIP, Regular) that decides which workflow steps a guest may perform |
| Ticket | The QR admission artefact issued to one guest; its status is `active`, `used`, or `invalidated` |
| Scan log | An append-only record of one scan of a ticket at one workflow step |
| System role / event role | A tenant-wide role held by a user, or a role held only on one event through an assignment |
| RSVP | A guest's confirm or decline response to an invitation |
| Blind index | A deterministic keyed hash stored next to encrypted data so that exact-match search still works |
