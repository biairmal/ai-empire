---
type: test-plan
id: NEW
title: "{{ Feature / release }} — Test Plan"
project: "{{ project-slug }}"
status: draft
version: 1
owner: "{{ Name, role }}"
created: "{{ YYYY-MM-DD }}"
updated: "{{ YYYY-MM-DD }}"
derived_from: ["{{ PRD-00X }}"]
references: []
---

# {{ Feature / release }} — Test Plan

<!--
Test Plan (structure based on ISO/IEC/IEEE 29119-3)
Answers: HOW do we prove the requirements are satisfied?
Every "Must" requirement in the PRD appears in the traceability table with at least one test.
Add `tested_by: [TP-00X]` to the PRD / design it covers. Replace every {{ … }} placeholder.
-->

## Introduction and Scope

<!-- What is being tested, for which release, and the objectives of testing. -->

{{ Scope and objectives of this test effort. }}

## Test Items

<!-- The software items under test, with versions/builds and repositories. -->

| Item | Version / build | Repository |
|------|-----------------|------------|
| {{ Component }} | {{ Version }} | {{ repo }} |

## Features to Be Tested

| Feature | Requirements |
|---------|--------------|
| {{ Feature }} | {{ FR-01, NFR-02 }} |

## Features Not to Be Tested

<!-- Explicit exclusions with reasons, so nobody assumes coverage that doesn't exist. -->

| Feature | Reason |
|---------|--------|
| {{ Feature }} | {{ Reason }} |

## Test Approach

<!-- Test levels and types, automation strategy, and tools. -->

| Level / type | Scope | Automated? | Tool | Responsible |
|--------------|-------|------------|------|-------------|
| Unit | {{ Scope }} | Yes | {{ Tool }} | Engineering |
| Integration | {{ Scope }} | {{ Yes / No }} | {{ Tool }} | {{ Team }} |
| End-to-end | {{ Scope }} | {{ Yes / No }} | {{ Tool }} | {{ Team }} |
| Performance | {{ Scope }} | {{ Yes / No }} | {{ Tool }} | {{ Team }} |
| Security | {{ Scope }} | {{ Yes / No }} | {{ Tool }} | {{ Team }} |
| User acceptance (UAT) | {{ Scope }} | No | — | {{ Client representative }} |

## Requirements Traceability

<!-- Requirements Traceability Matrix (RTM): every requirement → test cases → latest result. -->

| Requirement | Test cases | Status |
|-------------|------------|--------|
| {{ FR-01 }} | {{ TC-01, TC-02 }} | {{ Not run / Pass / Fail }} |

## Test Cases

<!-- One row per test case. Steps must be reproducible by someone who did not write them. -->

| ID | Title | Preconditions | Steps | Expected result | Priority |
|----|-------|---------------|-------|-----------------|----------|
| TC-01 | {{ Title }} | {{ Preconditions }} | {{ 1. … 2. … }} | {{ Expected result }} | {{ High / Medium / Low }} |

## Test Environment and Data

<!-- Environments, configuration, test accounts (never real passwords), and test data. Production data must be anonymised. -->

| Environment | URL | Data set | Notes |
|-------------|-----|----------|-------|
| {{ Staging }} | {{ URL }} | {{ Seed / anonymised copy }} | {{ Notes }} |

## Entry and Exit Criteria

**Entry criteria** (testing may start when):

- {{ e.g. Build deployed to staging and smoke tests pass }}

**Exit criteria** (testing is complete when):

- {{ e.g. All Must requirements pass; no open Critical/High defects }}

## Defect Management

<!-- Where defects are tracked, the severity scale, and the expected response time per severity. -->

| Severity | Definition | Response target |
|----------|------------|-----------------|
| Critical | {{ Definition }} | {{ Target }} |
| High | {{ Definition }} | {{ Target }} |
| Medium | {{ Definition }} | {{ Target }} |
| Low | {{ Definition }} | {{ Target }} |

**Tracker:** {{ Tool / project }}

## Risks and Contingencies

| Risk | Impact on testing | Contingency |
|------|-------------------|-------------|
| {{ Risk }} | {{ Impact }} | {{ Plan }} |

## Acceptance and Sign-off

<!-- Who formally accepts the release on behalf of each party. -->

| Role | Name | Decision | Date |
|------|------|----------|------|
| {{ Client product owner }} | {{ Name }} | {{ Accepted / rejected }} | {{ YYYY-MM-DD }} |
| {{ Delivery lead }} | {{ Name }} | {{ Accepted / rejected }} | {{ YYYY-MM-DD }} |
