---
type: implementation-plan
id: NEW
title: "{{ Feature name }} — Implementation Plan"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
derived_from: ["{{ TD-00X }}"]
references: []
---

# {{ Feature name }} — Implementation Plan

<!--
Implementation Plan
Answers: WHAT work will be done, in WHICH repository, in WHICH order?
Prerequisite: the Technical Design in `derived_from` must be approved.
The "Work Breakdown" section MUST contain exactly one ```yaml block in the format below;
the platform creates one task per entry. Replace every {{ … }} placeholder.
-->

## Overview

<!-- What will be built and how the work is split. -->

{{ Summary of the plan. }}

## Scope

**In scope:** {{ Items from the Technical Design delivered by this plan }}

**Out of scope:** {{ Items deferred, with reason }}

## Work Breakdown

<!--
One entry per task. Rules:
- key: short unique id (a-z, 0-9, -)
- repository: the name of one of the project's repositories
- title: imperative, under 72 characters (it becomes the commit message)
- description: everything a developer needs; reference requirement IDs
- depends_on: keys of tasks that must be merged first (optional)
Keep tasks small enough to review in one sitting (roughly under 400 changed lines).
-->

```yaml
tasks:
  - key: {{ api-endpoint }}
    repository: {{ backend }}
    title: {{ Add ticket validation endpoint }}
    description: {{ Implement FR-01 and FR-02 as described in TD-00X, including unit tests. }}
    depends_on: []
```

## Sequencing and Dependencies

<!-- Explain the order and any external dependencies (other teams, releases, data). -->

{{ Why tasks are ordered this way. }}

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| {{ Risk }} | {{ Mitigation }} |

## Definition of Done

<!-- Applies to every task in this plan. -->

- Code merged to the default branch after human approval
- Automated tests added or updated and passing
- {{ Documentation, API spec, or runbook updated where relevant }}
- {{ Any additional project-specific criteria }}
