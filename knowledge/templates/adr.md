---
type: adr
id: NEW
title: "{{ Short decision title in imperative form, e.g. Use PostgreSQL for ticket storage }}"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
decision_date: "{{ YYYY-MM-DD }}"
deciders: ["{{ Name }}"]
derived_from: []
supersedes: []
affects: []
references: []
---

# {{ Short decision title }}

<!--
Architecture Decision Record (MADR format)
Answers: WHY did we choose this solution?
One decision per record. Status lifecycle: draft (proposed) → approved (accepted) → superseded.
Never edit an approved ADR's decision; write a new ADR that `supersedes` it.
For ADRs stored at client, stack, or global scope, replace `project:` with `client:` or
`stack:` (or remove it for global).
-->

## Context and Problem Statement

<!-- Two or three sentences describing the situation and the question that needs a decision. -->

{{ Context and the question to be decided. }}

## Decision Drivers

<!-- The forces that matter: requirements, quality attributes, constraints, cost, team skills. -->

- {{ Driver }}

## Considered Options

- {{ Option 1 }}
- {{ Option 2 }}

## Decision Outcome

<!-- The chosen option and the key reason, tied to the decision drivers. -->

**Chosen option:** "{{ Option }}", because {{ justification }}.

## Consequences

<!-- What becomes easier or harder as a result. Include follow-up work. -->

- **Positive:** {{ Consequence }}
- **Negative:** {{ Consequence }}
- **Follow-up:** {{ Required follow-up, or "None" }}

## Pros and Cons of the Options

### {{ Option 1 }}

- Good, because {{ argument }}
- Bad, because {{ argument }}

### {{ Option 2 }}

- Good, because {{ argument }}
- Bad, because {{ argument }}

## Confirmation

<!-- How compliance with this decision is checked: code review rule, architecture test, linter, fitness function. -->

{{ How we verify the decision is followed. }}

## More Information

<!-- Optional: links, related decisions, and when this decision should be revisited. -->

{{ Links and revisit criteria, or "None". }}
