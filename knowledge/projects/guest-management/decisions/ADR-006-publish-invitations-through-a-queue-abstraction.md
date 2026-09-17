---
type: adr
id: ADR-006
title: "Publish invitations through a queue abstraction instead of sending messages directly"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-09-08"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [ARCH-001]
references: []
---

# Publish invitations through a queue abstraction instead of sending messages directly

<!-- Recorded from DEVELOPMENT_PLAN.md B8 and FEATURES.md#guests (commit 2026-09-08). -->

## Context and Problem Statement

Sending an invitation must eventually deliver an email or WhatsApp message, but no provider has been chosen. Message volume may be large (bulk invitations, thank-you messages). How does the API trigger delivery?

## Decision Drivers

- No provider has been chosen yet.
- Bulk sends must not block API requests.
- The backend should be switchable through configuration.

## Considered Options

- Publish an `InvitationMessage` to a go-sdk `queue.Publisher` (backends `noop`, `logging`, or `kafka`) behind a feature-local `InvitationPublisher` interface, and send best-effort
- Call an SMTP or WhatsApp provider synchronously from the API
- Use a database outbox table with a poller

## Decision Outcome

**Chosen option:** "Publish to a queue abstraction, best-effort", because it decouples the API from the undecided provider and works with no infrastructure by default (`QUEUE_BACKEND=noop`).

- The topic is `guests.invitation`, and messages are keyed by `guest_id`.
- A publish failure is logged and does not fail the invitation call. The guest's status and token are already saved.
- The invitation token is returned in the API response, because no message is delivered yet.

## Consequences

- **Positive:** The API is fast and independent of the provider. Kafka can be enabled by configuration.
- **Negative:** Delivery is not guaranteed; publishing is not transactional with the database write. Nothing consumes the messages yet, so no invitations are actually sent.
- **Follow-up:** Choose a provider and build a consumer that uses `message_templates` (PRD-001 Q6). Consider an outbox if lost messages become a problem. Post-event messages (B10) are expected to reuse this mechanism.

## Pros and Cons of the Options

### Queue abstraction

- Good, because it is asynchronous, the backend can be swapped, and tests can use noop.
- Bad, because a message can be lost between the database commit and the publish.

### Synchronous provider call

- Good, because it is simple, with an immediate result.
- Bad, because it is slow, couples the API to the provider, and handles bulk sends poorly.

### Outbox table

- Good, because delivery is guaranteed.
- Bad, because it needs a poller and extra tables before any delivery exists.

## Confirmation

The guests service tests call `SendInvitation` with a stub publisher that does nothing. No test yet asserts that a publish failure leaves the call successful, so that is a test gap. `QUEUE_BACKEND` is documented in `.env.example`.

## More Information

Revisit when the consumer is designed.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[ARCH-001-guest-management-architecture-overview|ARCH-001 · Guest Management — Architecture Overview]]
<!-- relations:end -->
