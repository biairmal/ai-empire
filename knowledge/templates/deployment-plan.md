---
type: deployment-plan
id: NEW
title: "{{ Release name / version }} — Deployment Plan"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
release: "{{ v1.2.0 }}"
planned_window: "{{ YYYY-MM-DD HH:MM–HH:MM TZ }}"
derived_from: []
depends_on: []
references: []
---

# {{ Release name / version }} — Deployment Plan

<!--
Deployment Plan (release / change implementation plan)
Answers: HOW is this release deployed safely, verified, and rolled back if needed?
Production deployment always requires explicit human approval. Never include secret values —
refer to secrets by name and location only. Replace every {{ … }} placeholder.
-->

## Release Summary

| Item | Value |
|------|-------|
| Release | {{ Version }} |
| Planned window | {{ Date, time, time zone }} |
| Expected downtime | {{ None / duration }} |
| Change owner | {{ Name }} |
| Risk level | {{ Low / Medium / High }} |

{{ What this release delivers, in one paragraph. }}

## Components and Versions

| Component | Repository | From version | To version | Artifact |
|-----------|------------|--------------|------------|----------|
| {{ API }} | {{ backend }} | {{ v1.1.0 }} | {{ v1.2.0 }} | {{ Image / package reference }} |

## Pre-Deployment Checklist

- [ ] Change approved by {{ approver }}
- [ ] Release notes published ({{ REL-00X }})
- [ ] Backups taken and verified at {{ time }}
- [ ] Stakeholders notified
- [ ] Rollback artifacts available
- [ ] {{ Project-specific check }}

## Deployment Steps

<!-- Numbered, copy-pasteable steps with expected duration and who performs each. -->

| # | Step | Command / action | Owner | Expected duration |
|---|------|------------------|-------|-------------------|
| 1 | {{ Step }} | `{{ command }}` | {{ Owner }} | {{ Minutes }} |

## Database Migrations

<!-- Migrations run, their order, lock impact, and whether they are reversible. Write "None" if no migrations. -->

| Migration | Reversible? | Lock / downtime impact |
|-----------|-------------|------------------------|
| {{ Migration }} | {{ Yes / No }} | {{ Impact }} |

## Configuration Changes

<!-- New or changed environment variables, feature flags, and secrets — by NAME only. -->

| Setting | Environment | Change | Stored in |
|---------|-------------|--------|-----------|
| {{ SETTING_NAME }} | {{ Production }} | {{ Added / changed }} | {{ Secret manager / config file }} |

## Verification

<!-- Smoke tests and checks that prove the release works, with who performs them. -->

| Check | How | Expected result | Owner |
|-------|-----|-----------------|-------|
| {{ Health endpoint }} | `{{ GET /health }}` | {{ 200 OK }} | {{ Owner }} |

## Rollback Plan

<!-- Clear trigger criteria, the exact rollback steps, and data considerations. Rollback must be rehearsed for High-risk releases. -->

**Rollback triggers:** {{ e.g. error rate > 2 % for 5 minutes, failed smoke test }}

| # | Step | Command / action | Owner |
|---|------|------------------|-------|
| 1 | {{ Step }} | `{{ command }}` | {{ Owner }} |

**Data considerations:** {{ How data written by the new version is handled on rollback }}

## Communication Plan

| When | Audience | Channel | Message owner |
|------|----------|---------|---------------|
| {{ Before window }} | {{ Users / client IT }} | {{ Email / status page }} | {{ Owner }} |
| {{ After completion }} | {{ Stakeholders }} | {{ Channel }} | {{ Owner }} |

## Post-Deployment Monitoring

<!-- What to watch, for how long, and who is on call. -->

| Signal | Dashboard / query | Normal range | Watch period |
|--------|-------------------|--------------|--------------|
| {{ Error rate }} | {{ Link }} | {{ < 1 % }} | {{ 24 h }} |

**On call:** {{ Name / rotation }}

## Sign-off

| Role | Name | Decision | Date |
|------|------|----------|------|
| {{ Change approver }} | {{ Name }} | {{ Approved / rejected }} | {{ YYYY-MM-DD }} |
| {{ Client IT representative }} | {{ Name }} | {{ Approved / rejected }} | {{ YYYY-MM-DD }} |
