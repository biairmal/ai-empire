---
type: prd
id: NEW
title: "{{ Feature or product name }}"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
derived_from: []
depends_on: []
references: []
---

# {{ Feature or product name }}

<!--
Product Requirements Document (PRD)
Answers: WHAT problem are we solving, for WHOM, and WHY — not HOW (that belongs in the Technical Design).
Write for a business reader. Replace every {{ … }} placeholder. If a section does not apply,
write "Not applicable" and one sentence explaining why. Guidance comments like this one are
removed automatically when the document is exported.
-->

## Summary

<!-- 3–5 sentences a busy executive can read on their own: the problem, the proposed outcome, who benefits, and the expected impact. -->

{{ One-paragraph summary of the problem, the outcome, and the benefit. }}

## Background and Problem Statement

<!-- Why now? Describe the current situation, the pain it causes, and the evidence (support tickets, metrics, customer feedback, regulation). Avoid proposing solutions here. -->

**Current situation:** {{ How things work today. }}

**Problem:** {{ What goes wrong, for whom, how often. }}

**Evidence:** {{ Data, quotes, incidents, or requirements that prove the problem is real. }}

## Goals and Success Metrics

<!-- Goals are outcomes, not features. Each metric needs a baseline and a target so success can be measured after release. -->

| # | Goal | Metric | Baseline | Target | Measured by |
|---|------|--------|----------|--------|-------------|
| G1 | {{ Outcome }} | {{ Metric }} | {{ Today }} | {{ Target and date }} | {{ Source / dashboard }} |

## Non-Goals

<!-- What is explicitly OUT of scope for this release. Non-goals prevent scope creep and set client expectations. -->

- {{ Out-of-scope item and, if helpful, when it may be reconsidered. }}

## Users and Personas

<!-- Who uses this? Keep personas short and grounded in real user groups. -->

| Persona | Description | Primary needs | Frequency of use |
|---------|-------------|---------------|------------------|
| {{ Persona name }} | {{ Who they are }} | {{ What they need }} | {{ Daily / weekly / … }} |

## User Stories

<!-- Format: "As a <persona>, I want <capability> so that <benefit>." Each story gets acceptance criteria in Given/When/Then form. Reference stories from Functional Requirements. -->

### US-01: {{ Short story title }}

As a **{{ persona }}**, I want **{{ capability }}** so that **{{ benefit }}**.

**Acceptance criteria**

- Given {{ context }}, when {{ action }}, then {{ observable result }}.

## Functional Requirements

<!-- Numbered, testable, unambiguous statements of what the system must do. Priority uses MoSCoW: Must, Should, Could, Won't (this time). Every "Must" needs at least one acceptance criterion. -->

| ID | Requirement | Priority | User story | Acceptance criteria |
|----|-------------|----------|------------|---------------------|
| FR-01 | The system shall {{ behaviour }}. | Must | US-01 | {{ Verifiable criterion }} |

## Non-Functional Requirements

<!-- Quality attributes (ISO/IEC 25010): performance, availability, security, privacy, accessibility, usability, compatibility, maintainability, compliance. Make each one measurable. -->

| ID | Category | Requirement | Target / threshold |
|----|----------|-------------|--------------------|
| NFR-01 | Performance | {{ Requirement }} | {{ e.g. p95 < 300 ms at 100 req/s }} |
| NFR-02 | Security | {{ Requirement }} | {{ Measurable target }} |
| NFR-03 | Accessibility | {{ Requirement }} | {{ e.g. WCAG 2.2 AA }} |

## Assumptions and Constraints

<!-- Assumptions are believed true but unverified (list how to verify). Constraints are fixed limits: budget, deadline, technology, regulation, contracts. -->

**Assumptions**

- {{ Assumption }} — *to be verified by {{ who / how }}*

**Constraints**

- {{ Constraint }}

## Dependencies

<!-- Other teams, systems, vendors, data, or decisions this work relies on. -->

| Dependency | Owner | Needed by | Status |
|------------|-------|-----------|--------|
| {{ System / team / vendor }} | {{ Owner }} | {{ Date or milestone }} | {{ Confirmed / pending }} |

## Risks

<!-- Product and delivery risks. Likelihood and impact: Low / Medium / High. -->

| Risk | Likelihood | Impact | Mitigation | Owner |
|------|------------|--------|------------|-------|
| {{ Risk }} | {{ L/M/H }} | {{ L/M/H }} | {{ Mitigation }} | {{ Owner }} |

## Release Scope

<!-- What ships in which release or milestone. Link to the Implementation Plan once it exists. -->

| Milestone | Scope (requirements) | Target date |
|-----------|----------------------|-------------|
| {{ MVP / Phase 1 }} | {{ FR-01, FR-02 }} | {{ YYYY-MM-DD }} |

## Open Questions

<!-- Unresolved decisions. Each needs an owner and a due date. Resolve all "blocking" questions before approval. -->

| # | Question | Owner | Due | Blocking? |
|---|----------|-------|-----|-----------|
| Q1 | {{ Question }} | {{ Owner }} | {{ Date }} | {{ Yes / No }} |

## Glossary

<!-- Optional. Domain terms a client reader may not know. -->

| Term | Definition |
|------|------------|
| {{ Term }} | {{ Definition }} |
