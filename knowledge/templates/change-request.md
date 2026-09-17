---
type: change-request
id: NEW
title: "{{ Short description of the change }}"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
requested_by: "{{ Name, organisation }}"
priority: "{{ Low / Medium / High / Urgent }}"
affects: ["{{ PRD-00X }}"]
references: []
---

# CR: {{ Short description of the change }}

<!--
Change Request
Answers: WHAT needs to change in already-approved work, WHY, and WHAT does it affect?
Approved documents are never edited silently. An approved CR authorises new versions of the
documents listed in `affects`; each new version sets `derived_from: [CR-00X]`.
The platform adds an automatic impact analysis to the approval request.
Replace every {{ … }} placeholder.
-->

## Summary

{{ One paragraph: what changes and the expected outcome. }}

## Reason for Change

<!-- Business, regulatory, technical, or user-feedback driver. Include evidence. -->

{{ Why the change is needed now. }}

## Current Behaviour

<!-- What the approved documents and the system say or do today. Cite document IDs and requirement IDs. -->

{{ Current behaviour, with references (e.g. PRD-001 FR-03). }}

## Proposed Change

<!-- The new behaviour, precisely enough to update the affected documents. -->

{{ Proposed behaviour. }}

## Affected Artifacts

<!-- Documents, requirements, components, and repositories affected. Keep this consistent with `affects` in the front matter. -->

| Artifact | Type | Change needed |
|----------|------|---------------|
| {{ PRD-00X }} | {{ PRD }} | {{ Update FR-03 }} |

## Impact Assessment

| Dimension | Impact | Notes |
|-----------|--------|-------|
| Scope | {{ None / Low / Medium / High }} | {{ Notes }} |
| Schedule | {{ Impact }} | {{ e.g. +3 days }} |
| Cost / effort | {{ Impact }} | {{ Estimate }} |
| Quality / risk | {{ Impact }} | {{ Notes }} |
| Users / operations | {{ Impact }} | {{ Notes }} |
| Security / compliance | {{ Impact }} | {{ Notes }} |

## Options Considered

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A | {{ Implement as proposed }} | {{ Pros }} | {{ Cons }} |
| B | {{ Alternative or "do nothing" }} | {{ Pros }} | {{ Cons }} |

## Recommendation

{{ Recommended option and justification. }}

## Rollback and Contingency

<!-- How to revert if the change causes problems after release. -->

{{ Rollback approach. }}
