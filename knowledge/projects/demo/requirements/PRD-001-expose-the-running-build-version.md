---
type: prd
id: PRD-001
title: "Expose the Running Build Version"
project: demo
status: rejected
version: 1
owner: "Product Manager, Demo API"
created: "2026-09-17"
updated: "2026-09-17"
derived_from: []
depends_on: []
references: []
---

# Expose the Running Build Version

## Summary

Operators and support staff currently have no reliable way to tell which build of the Demo API is running in a given environment. When a bug is reported, support cannot confirm which release it came from, which slows triage and can lead to fixes being tested against the wrong version. This PRD proposes exposing the running build version of the Demo API over HTTP so that anyone with access can read it directly from the service. The result is faster, more accurate matching of bug reports to releases and less guesswork during incidents.

## Background and Problem Statement

**Current situation:** The Demo API is deployed to one or more environments, but the service does not report which build it is running. To find out, staff must infer it from deployment records or ask whoever performed the release.

**Problem:** Operators and support engineers cannot confirm the running build of the Demo API on demand. When a bug is reported, they cannot reliably tell which release produced the reported behaviour, so triage is slower and fixes risk being validated against the wrong version. This affects the operations and support teams during routine support and, more acutely, during incidents.

**Evidence:** The originating request states that operators want to know which build is running so support can match bug reports to releases. No further metrics (ticket volume, time-to-triage) were supplied with the request; the scale of the problem is captured as an open question (Q3) rather than stated as fact.

## Goals and Success Metrics

| # | Goal | Metric | Baseline | Target | Measured by |
|---|------|--------|----------|--------|-------------|
| G1 | Support can identify the running build of any Demo API environment on demand | Build version readable over HTTP from a running instance | Not available today | Available in every deployed environment | Manual check against each environment |
| G2 | Bug reports can be matched to a specific release | Share of bug reports for the Demo API that record a verifiable build version | Not currently measured (see Q3) | Proposed: majority of new reports record it within one release cycle — target to be confirmed (see Q4) | Support ticketing records |

## Non-Goals

- A user interface, dashboard, or admin console for viewing the version — the value is exposed over HTTP only this time; a UI may be reconsidered if demand emerges.
- Exposing build or version information for services other than the Demo API.
- Reporting detailed build metadata such as dependency lists, environment configuration, or change logs — only what is needed to identify the running build.
- Historical or multi-version reporting — the service reports the build it is currently running, not past builds.

## Users and Personas

| Persona | Description | Primary needs | Frequency of use |
|---------|-------------|---------------|------------------|
| Operator | Runs and monitors deployed Demo API environments | Confirm which build is running in a given environment | As needed, and during every incident |
| Support engineer | Triages and responds to reported bugs | Match a reported bug to the release it came from | Per bug report |
| Developer | Diagnoses and fixes reported issues | Reproduce and verify fixes against the correct build | During investigation and verification |

## User Stories

### US-01: Operator confirms the running build

As an **operator**, I want **to read the build version from a running Demo API instance** so that **I can confirm which build is deployed in an environment without asking whoever released it**.

**Acceptance criteria**

- Given a running Demo API instance, when I request the build version over HTTP, then the response returns the version of the build currently running.
- Given two environments running different builds, when I request the build version from each, then each returns its own build version.

### US-02: Support engineer matches a bug report to a release

As a **support engineer**, I want **the running build version to identify exactly one release**, so that **I can match a reported bug to the release that produced it**.

**Acceptance criteria**

- Given a build version obtained from the API, when I look it up, then it corresponds to exactly one released build.
- Given a bug report, when it includes the build version, then I can determine the release under investigation without ambiguity.

## Functional Requirements

| ID | Requirement | Priority | User story | Acceptance criteria |
|----|-------------|----------|------------|---------------------|
| FR-01 | The Demo API shall expose the version of the build it is currently running over HTTP. | Must | US-01 | A request over HTTP returns the running build version. |
| FR-02 | The reported build version shall unambiguously identify a single released build. | Must | US-02 | The returned value maps to exactly one released build. |
| FR-03 | The reported build version shall reflect the build actually running, updating when a new build is deployed. | Must | US-01 | After deploying a different build, the reported version changes to match it. |
| FR-04 | The build version shall be returned in a form that is both human-readable and reliably machine-parsable, so it can be copied into a bug report or read by tooling. | Should | US-02 | The response contains a clearly identifiable version value that tooling can extract without scraping free text. |
| FR-05 | The response shall exclude secrets, internal file paths, and sensitive configuration. | Must | US-02 | The response contains only build-identifying information and no sensitive data. |

## Non-Functional Requirements

| ID | Category | Requirement | Target / threshold |
|----|----------|-------------|--------------------|
| NFR-01 | Performance | Requesting the build version shall be lightweight and shall not depend on downstream systems. | Served directly by the API; proposed p95 < 300 ms — threshold to be confirmed with the delivery team. |
| NFR-02 | Availability | The build version shall be readable whenever the Demo API instance is running. | Available for the life of the running process. |
| NFR-03 | Security | The build version response shall not expose secrets, credentials, internal paths, or sensitive configuration. | Zero sensitive fields in the response. |
| NFR-04 | Accessibility | Not applicable — this feature exposes a machine-facing HTTP response and has no user-facing interface, so WCAG criteria do not apply. | Not applicable. |
| NFR-05 | Maintainability | The reported version shall be established as part of the build/release process rather than edited by hand, so it cannot drift from the actual build. | Version set automatically at build time. |

## Assumptions and Constraints

**Assumptions**

- The build/release process can produce a version value that uniquely identifies each released build — *to be verified by the delivery team*.
- Whoever can reach the Demo API over HTTP is permitted to see its build version; if not, an access control decision is required (see Q1) — *to be verified with the security owner*.
- A single version value per running instance is sufficient for support to match a report to a release — *to be verified with the support team*.

**Constraints**

- The build version must be exposed over HTTP, per the originating request.
- The solution must comply with the project's Security Principles (GL-002), in particular that responses expose no secrets and that access is authenticated and authorised where required.

## Dependencies

| Dependency | Owner | Needed by | Status |
|------------|-------|-----------|--------|
| Build/release process supplies a unique build version at build time | Delivery team | Before implementation | Pending |
| Decision on whether the version response requires authentication (Q1) | Security owner | Before implementation | Pending |
| Definition of what the version value contains (Q2) | Architect / delivery team | Before implementation | Pending |

## Risks

| Risk | Likelihood | Impact | Mitigation | Owner |
|------|------------|--------|------------|-------|
| The version response inadvertently exposes sensitive information (paths, config, secrets) | Low | High | Restrict the response to a build identifier only (FR-05, NFR-03); review before release | Product Manager / Security owner |
| Exposing the build version to unauthenticated callers reveals more than intended | Medium | Medium | Resolve the authentication decision (Q1) before release | Security owner |
| The reported version drifts from the build actually running | Low | Medium | Set the version automatically at build time (NFR-05, FR-03) | Delivery team |
| The version value cannot be traced back to a single release | Low | High | Confirm a unique, release-traceable identifier (FR-02, Q2) | Delivery team |

## Release Scope

| Milestone | Scope (requirements) | Target date |
|-----------|----------------------|-------------|
| MVP | FR-01, FR-02, FR-03, FR-05, NFR-01, NFR-02, NFR-03, NFR-05 | To be set by the delivery team |
| Follow-up | FR-04 (machine-parsable form) if not delivered in the MVP | To be set by the delivery team |

## Open Questions

| # | Question | Owner | Due | Blocking? |
|---|----------|-------|-----|-----------|
| Q1 | Must the build version response be restricted to authenticated/authorised callers, or may it be publicly readable? | Security owner | Before implementation | Yes |
| Q2 | What exactly should the build version contain (e.g. release number, source revision, build number, build timestamp)? | Architect / delivery team | Before implementation | Yes |
| Q3 | Are bug reports for the Demo API currently recording a build version at all, and if so how often? (Baseline for G2.) | Support team | Before approval | No |
| Q4 | What adoption target and date should G2 commit to? | Product Manager / Support team | Before approval | No |

## Glossary

| Term | Definition |
|------|------------|
| Build | A specific compiled/packaged instance of the Demo API produced by the release process. |
| Build version | A value that identifies which build is running, used to match it to a release. |
| Release | A build that has been made available for deployment. |
| Operator | A person who runs and monitors deployed Demo API environments. |
