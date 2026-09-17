---
type: database-design
id: NEW
title: "{{ Domain / feature }} — Database Design"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
database: "{{ PostgreSQL 17 / MySQL 8 / … }}"
implements: []
depends_on: []
references: []
---

# {{ Domain / feature }} — Database Design

<!--
Database Design
Answers: HOW is the data represented and constrained?
Covers structure, integrity, performance, privacy, and recovery. Migrations in the code
repository must match this document. Replace every {{ … }} placeholder.
-->

## Overview

<!-- Which data this covers, which service owns it, and the database engine and version. -->

{{ Scope of the data model and owning service. }}

## Data Model

<!-- Entity-relationship diagram. Mermaid erDiagram renders in most Markdown viewers. -->

```mermaid
erDiagram
  {{ PARENT }} ||--o{ {{ CHILD }} : "{{ has }}"
```

## Entities

<!-- One table per entity. Mark personal data (PII) in the Classification column: Public, Internal, Confidential, PII, Sensitive PII. -->

### {{ table_name }}

{{ What one row represents. }}

| Column | Type | Null | Default | Constraints | Classification | Description |
|--------|------|------|---------|-------------|----------------|-------------|
| id | {{ uuid / bigint }} | No | {{ default }} | PK | Internal | Primary key |
| {{ column }} | {{ type }} | {{ Yes / No }} | {{ default }} | {{ FK / UNIQUE / CHECK }} | {{ Classification }} | {{ Description }} |

## Relationships and Constraints

<!-- Foreign keys with ON DELETE behaviour, unique rules, check constraints, and business invariants enforced in the database. -->

| Constraint | Tables | Rule | On delete |
|------------|--------|------|-----------|
| {{ fk_name }} | {{ child → parent }} | {{ Rule }} | {{ CASCADE / RESTRICT / SET NULL }} |

## Indexes and Performance

<!-- Indexes with the query they serve, expected data volumes and growth, and heavy queries. -->

| Index | Table | Columns | Serves query | Type |
|-------|-------|---------|--------------|------|
| {{ idx_name }} | {{ table }} | {{ columns }} | {{ Query / endpoint }} | {{ btree / partial / unique }} |

| Table | Rows today | Growth per month | Notes |
|-------|------------|------------------|-------|
| {{ table }} | {{ count }} | {{ growth }} | {{ Partitioning / archiving }} |

## Migrations and Backfill

<!-- Ordered migrations, backfill strategy for existing data, locking/downtime impact, and down-migration behaviour. -->

| # | Migration | Locks / downtime | Reversible? |
|---|-----------|------------------|-------------|
| 1 | {{ Description }} | {{ Impact }} | {{ Yes / No — why }} |

## Data Privacy and Retention

<!-- Personal data held, legal basis (e.g. GDPR / local law), retention periods, deletion/anonymisation, encryption at rest, and who can access it. -->

| Data | Purpose | Retention | Deletion method | Encrypted? |
|------|---------|-----------|-----------------|------------|
| {{ Data }} | {{ Purpose }} | {{ Period }} | {{ Method }} | {{ Yes / No }} |

## Backup and Recovery

<!-- Backup frequency, retention, recovery point objective (RPO), recovery time objective (RTO), and the last successful restore test. -->

| Item | Value |
|------|-------|
| Backup method and frequency | {{ e.g. nightly pg_dump + WAL archiving }} |
| Retention | {{ Period }} |
| RPO / RTO | {{ e.g. 15 min / 1 h }} |
| Last restore test | {{ YYYY-MM-DD, result }} |

## Open Questions

| # | Question | Owner | Due | Blocking? |
|---|----------|-------|-----|-----------|
| Q1 | {{ Question }} | {{ Owner }} | {{ Date }} | {{ Yes / No }} |
