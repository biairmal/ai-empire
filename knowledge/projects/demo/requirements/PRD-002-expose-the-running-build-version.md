---
type: prd
id: PRD-002
title: "Expose the Running Build Version"
project: demo
status: approved
version: 1
owner: "Product Manager, Demo API"
created: "2026-09-17"
updated: "2026-09-17"
derived_from: []
depends_on: []
references:
  - GL-001
  - GL-002
---

# Expose the Running Build Version

## Summary

The Demo API does not currently report which build of the software is running in a given environment. When a user reports a bug, support cannot reliably tell which release that report belongs to, which slows triage and risks misdiagnosis. This document requires the Demo API to expose its running build version over HTTP so that any authorized caller can read it. The benefit is faster, more accurate matching of bug reports to the release that produced them.

## Background and Problem Statement

**Current situation:** The Demo API is deployed to one or more environments, but it provides no interface that reports its own build or version. To learn which build is live, someone must inspect the deployment pipeline or the running infrastructure directly.

**Problem:** Support engineers and operators cannot confirm which build is serving a given environment. As a result, incoming bug reports cannot be tied to a specific release, which wastes triage effort and can lead to fixes being applied against the wrong build. The frequency and cost of this problem have not been measured (see Open Questions).

**Evidence:** The request originates from operators who need to match bug reports to releases. No quantitative data (triage time, incident volume) is available at the time of writing; these are recorded as an assumption and an open question rather than stated as fact.

## Goals and Success Metrics

| # | Goal | Metric | Baseline | Target | Measured by |
|---|------|--------|----------|--------|-------------|
| G1 | Support can identify the running build for any incident | Share of incidents where the running build version is recorded | 0% — not exposed today | 100% of incidents in environments where the feature is deployed, within one release cycle after launch | Incident / support ticket records |
| G2 | Operators can confirm the deployed build without inspecting infrastructure | Share of release verifications done via the exposed version rather than pipeline/infrastructure inspection | 0% today | Majority of verifications (specific target to be confirmed — see Q1) | Operator confirmation records |

## Non-Goals

- Exposing build metadata beyond the version identifier (for example a full dependency manifest or commit history); may be reconsidered if a concrete need emerges.
- Providing a user interface or dashboard for browsing versions; the version is consumed programmatically.
- Reporting versions for services other than the Demo API.
- Changing the existing authentication or authorization model.

## Users and Personas

| Persona | Description | Primary needs | Frequency of use |
|---------|-------------|---------------|------------------|
| Support engineer | Handles bug reports from users of the Demo API | Read the running build version to match a report to a release | Per incident |
| Operator / SRE | Deploys and runs the Demo API | Confirm which build is live in an environment | Per release and as needed |

## User Stories

### US-01: Support reads the running build version

As a **support engineer**, I want **to read the build version the Demo API is currently running** so that **I can match a bug report to the release that produced it**.

**Acceptance criteria**

- Given a running Demo API instance, when an authorized caller requests the build version over HTTP, then the response contains the version of the build currently serving that instance.

### US-02: Operator verifies the deployed build

As an **operator**, I want **to confirm the running build version after a deployment** so that **I can verify the intended release is live without inspecting the pipeline or infrastructure**.

**Acceptance criteria**

- Given a completed deployment, when the operator requests the build version over HTTP, then the returned version matches the release that was deployed.

## Functional Requirements

| ID | Requirement | Priority | User story | Acceptance criteria |
|----|-------------|----------|------------|---------------------|
| FR-01 | The system shall expose the running build version over HTTP to authorized callers. | Must | US-01, US-02 | A caller receives the version of the currently running build in an HTTP response. |
| FR-02 | The reported version shall uniquely identify the build so it can be matched to a specific release. | Must | US-01 | The reported value corresponds one-to-one to a released build. |
| FR-03 | The reported version shall reflect the build actually running, not a stale or hard-coded value. | Should | US-02 | After a new build is deployed, the reported version changes to that build's version. |

## Non-Functional Requirements

| ID | Category | Requirement | Target / threshold |
|----|----------|-------------|--------------------|
| NFR-01 | Performance | Reporting the version shall not perceptibly affect service performance. | No measurable impact on request latency; a numeric threshold to be confirmed with the Architect (see Q4). |
| NFR-02 | Security | Access to the version shall follow existing API policy, and the response shall reveal only the build version — no secrets or additional sensitive detail (GL-002). | Authenticated/authorized access per policy; response limited to the version identifier. |
| NFR-03 | Accessibility | Not applicable — the version is consumed programmatically over HTTP and is not presented through a user interface. | Not applicable. |

## Assumptions and Constraints

**Assumptions**

- A build/version identifier is available at build time and can be embedded in the deployed artifact — *to be verified by the Architect / build pipeline owner*.
- Each environment runs a single build at a time, so one reported version is unambiguous — *to be verified by Operations*.
- Support's workflow can record the version once it is retrievable — *to be verified by the Support lead*.

**Constraints**

- Must comply with the Security Principles (GL-002): authenticated and authorized access, and no secret leakage.
- Must comply with the Engineering Principles (GL-001), including least privilege and simple over clever.
- Scope is limited to the Demo API.

## Dependencies

| Dependency | Owner | Needed by | Status |
|------------|-------|-----------|--------|
| Build pipeline injects a unique version identifier into the deployed artifact | Delivery / build pipeline owner | Implementation start | Pending |

## Risks

| Risk | Likelihood | Impact | Mitigation | Owner |
|------|------------|--------|------------|-------|
| Exposing the version aids attacker fingerprinting of the running build | Medium | Medium | Require authorized access per policy (GL-002); expose only the version identifier | Architect / Security |
| Reported version is stale and does not match the running build, causing wrong release matching | Low | Medium | Derive the version automatically from the build artifact rather than a manually maintained value | Architect |

## Release Scope

| Milestone | Scope (requirements) | Target date |
|-----------|----------------------|-------------|
| MVP | FR-01, FR-02 | To be set by the delivery team (see Q1) |
| Follow-up | FR-03 | To be set by the delivery team |

## Open Questions

| # | Question | Owner | Due | Blocking? |
|---|----------|-------|-----|-----------|
| Q1 | What are the target dates and the baseline for triage time / incident volume this feature should improve? | Product Manager / Support lead | Before approval | No |
| Q2 | Must the version be restricted to authorized callers, or may it be public? | Architect / Security | Before approval | Yes |
| Q3 | What is the version identifier's format and source (for example semantic version, build number, or commit reference)? | Architect | Design phase | No |
| Q4 | What performance threshold applies to reporting the version? | Architect | Design phase | No |
| Q5 | Which environments must expose the version (production only, or all environments)? | Operations lead | Before approval | No |

## Glossary

| Term | Definition |
|------|------------|
| Build version | The identifier that uniquely designates a specific build of the Demo API. |
| Running build | The build currently serving requests in a given environment. |
| Release | A build that has been promoted and deployed to an environment for use. |
</content>
</invoke>

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **References:** [[engineering-principles|GL-001 · Engineering Principles]], [[security-principles|GL-002 · Security Principles]]
<!-- relations:end -->
