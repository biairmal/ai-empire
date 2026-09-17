---
type: runbook
id: NEW
title: "{{ Service name }} — Operations Runbook"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
documents: []
references: []
---

# {{ Service name }} — Operations Runbook

<!--
Operations Runbook
Answers: HOW is the system operated, monitored, and recovered?
Written for an on-call engineer at 3 a.m. who has never seen the system: short steps,
exact commands, expected output. Never include secret values. Replace every {{ … }} placeholder.
-->

## Service Overview

| Item | Value |
|------|-------|
| Service | {{ Name }} |
| Business function | {{ What breaks for users if this is down }} |
| Criticality | {{ Tier 1 / 2 / 3 }} |
| Service level objective | {{ e.g. 99.9 % monthly availability }} |
| Owning team | {{ Team }} |

## Architecture and Dependencies

<!-- Components and the upstream/downstream services this depends on, with the effect of each being unavailable. -->

| Dependency | Type | If unavailable |
|------------|------|----------------|
| {{ Database }} | {{ Internal / external }} | {{ Effect and fallback }} |

## Environments and Access

<!-- How to reach each environment and who can grant access. Credentials by NAME and location only. -->

| Environment | URL / host | How to get access | Credentials stored in |
|-------------|------------|-------------------|-----------------------|
| Production | {{ URL }} | {{ Process }} | {{ Secret manager entry name }} |

## Monitoring and Alerts

| Alert | Meaning | Severity | First response |
|-------|---------|----------|----------------|
| {{ Alert name }} | {{ What it indicates }} | {{ P1–P4 }} | {{ Link to procedure below }} |

**Dashboards:** {{ Links }}

## Routine Procedures

<!-- Regular tasks: deploy, restart, scale, rotate credentials, renew certificates, clear queues. -->

### {{ Procedure name }}

1. {{ Step with exact command }}
2. {{ Step }}

**Expected result:** {{ What success looks like }}

## Troubleshooting

<!-- Symptom → likely cause → diagnosis → fix. Start with the most common problems. -->

| Symptom | Likely cause | How to confirm | Fix |
|---------|--------------|----------------|-----|
| {{ Symptom }} | {{ Cause }} | `{{ command }}` | {{ Action }} |

## Incident Response and Escalation

<!-- Severity definitions, who to call, in what order, and communication expectations. -->

| Severity | Definition | Response time | Escalate to |
|----------|------------|---------------|-------------|
| P1 | {{ Full outage / data loss }} | {{ 15 min }} | {{ Role / person }} |
| P2 | {{ Major degradation }} | {{ 1 h }} | {{ Role / person }} |

**Status updates:** {{ Channel and frequency }}

## Backup and Restore

| Item | Value |
|------|-------|
| Backup schedule | {{ Schedule }} |
| Backup location | {{ Location }} |
| RPO / RTO | {{ Values }} |

**Restore procedure**

1. {{ Step with exact command }}

**Last restore test:** {{ YYYY-MM-DD, result }}

## Contacts

| Role | Name | Contact | Hours |
|------|------|---------|-------|
| {{ Primary on-call }} | {{ Name }} | {{ Phone / chat }} | {{ Hours, time zone }} |
| {{ Client IT contact }} | {{ Name }} | {{ Contact }} | {{ Hours }} |
