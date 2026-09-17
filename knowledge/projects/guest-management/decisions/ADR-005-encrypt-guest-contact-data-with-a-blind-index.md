---
type: adr
id: ADR-005
title: "Encrypt guest email and phone at rest with AES-256-GCM and search them through an HMAC blind index"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Architect"
created: "2026-09-17"
updated: "2026-09-17"
decision_date: "2026-09-06"
deciders: ["Bandana Irmal A"]
derived_from: []
supersedes: []
affects: [DB-001, API-001]
references: []
---

# Encrypt guest email and phone at rest with AES-256-GCM and search them through an HMAC blind index

<!-- Recorded from DEVELOPMENT_PLAN.md B8 Technical Design and FEATURES.md#guests (commit 2026-09-06). -->

## Context and Problem Statement

Guests are not users, but their contact details are personal data held for tenants. The product owner wants no raw guest PII in the database, while staff still need to search guests. How is guest contact data stored and searched?

## Decision Drivers

- No plaintext email or phone at rest (REQUIREMENT.md §5.4).
- Staff must be able to search by name (including partial matches) and look up an exact email or phone.
- The encryption scheme may change later (another cipher, a KMS), so the service code must not depend on it.
- The app should never implement ciphers itself; they belong in `go-sdk`.

## Considered Options

- Encrypt email and phone with AES-256-GCM, keep the name in plaintext, and store deterministic HMAC-SHA256 blind indexes (`email_hash`, `phone_hash`) for exact match
- Deterministic encryption of email and phone
- Database or disk-level encryption only
- Encrypt the name as well

## Decision Outcome

**Chosen option:** "AES-256-GCM plus an HMAC blind index, with the name in plaintext", because it removes plaintext contact data while keeping exact-match lookup and partial name search.

- The feature-local `PIIEncryptor` interface (`Encrypt`, `Decrypt`, `BlindIndex`) is implemented in `internal/app` by an adapter over go-sdk `crypto`.
- There are two separate keys, `GUEST_PII_ENCRYPTION_KEY` and `GUEST_PII_BLIND_INDEX_KEY` (base64), with no defaults.
- Values are normalised before hashing: email is lowercased and trimmed, and phone keeps only digits.
- A list filter on `email` or `phone` must be an exact match. `;like` on them returns 400.

## Consequences

- **Positive:** A database dump reveals no guest email or phone. The encryption scheme can be swapped in one adapter.
- **Negative:** Email and phone can't be searched partially. Losing a key makes the data unrecoverable, and rotating the blind-index key requires re-hashing every row.
- **Follow-up:** Define key custody, backup, and a rotation procedure (PRD-001 Q9). Only `guests` uses this today. Extract it into `internal/core` if a second consumer appears.

## Pros and Cons of the Options

### AES-GCM plus a blind index

- Good, because encryption is semantically secure and exact lookup still works.
- Bad, because there are two keys to manage and no partial match.

### Deterministic encryption

- Good, because a single column can serve equality search.
- Bad, because identical values produce identical ciphertext, which leaks equality and frequency.

### Database-level encryption only

- Good, because it needs no code.
- Bad, because the data is plaintext to anyone with database access, which does not meet the requirement.

### Encrypt the name as well

- Good, because it protects the most data.
- Bad, because partial name search would no longer be possible.

## Confirmation

`guest_service__test.go` covers rejecting `;like` on email and phone and rewriting the filter to the hash column. The mock `PIIEncryptor` is generated with mockgen.

## More Information

Revisit if a KMS becomes available, or when retention or deletion rules are defined (PRD-001 Q11).

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Affects:** [[DB-001-guest-management-database-design|DB-001 · Guest Management — Database Design]], [[API-001-guest-management-api-specification|API-001 · Guest Management REST API — API Specification]]
<!-- relations:end -->
