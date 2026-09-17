# Documents Guide

What each engineering document is for, when you need it, and how it moves from draft to an approved, client-ready file.

You don't need product-management experience to use this. The templates tell the author (you or an AI agent) what to write, and the validator refuses anything incomplete.

- Templates: [knowledge/templates/](../knowledge/templates/)
- Contracts (the rules each type must follow): [knowledge/contracts/](../knowledge/contracts/)
- How requests produce documents automatically: [workflows.md](workflows.md)

---

## 1. Which document do I need?

Each document answers **one** question. If you're unsure, find your question in the table.

| Question | Document | ID | Folder | Written by | Approval |
|----------|----------|----|--------|------------|----------|
| What problem are we solving, for whom, and why? | **PRD** (Product Requirements Document) | `PRD-001` | `requirements/` | Product Manager agent or you | Required |
| How should the user experience it? | **UX / UI Specification** | `UX-001` | `ux/` | You or a designer | Required |
| How is the whole system built, and why? | **Architecture Overview** | `ARCH-001` | `architecture/` | You or the Architect agent | Required |
| How will we build this feature? | **Technical Design** | `TD-001` | `architecture/` | Architect agent | Required |
| Why did we choose this option? | **ADR** (Architecture Decision Record) | `ADR-001` | `decisions/` | Architect agent or you | Required |
| What contract does the API expose? | **API Specification** | `API-001` | `api/` | You or the Architect agent | Required |
| How is the data stored and protected? | **Database Design** | `DB-001` | `database/` | You or the Architect agent | Required |
| How do we prove it works? | **Test Plan** | `TP-001` | `testing/` | You | Required |
| What work, in which repository, in what order? | **Implementation Plan** | `PLAN-001` | `planning/` | Planner agent | Optional (the `feature` workflow asks for it) |
| How do we release it safely? | **Deployment Plan** | `DEP-001` | `operations/` | You | Required |
| How do we run and fix it in production? | **Operations Runbook** | `RB-001` | `operations/` | You | Optional |
| What needs to change in approved work? | **Change Request** | `CR-001` | `changes/` | Product Manager agent or you | Required |
| What changed in this release? | **Release Notes** | `REL-001` | `releases/` | You | Optional |
| Which tools and conventions does this project use? | **Guideline** | `GL-001` | `guidelines/` | You | Required (loaded into every task of the project) |

Folders are relative to `knowledge/projects/<project>/`. IDs are numbered per project, so every project has its own `PRD-001`.

### The typical order

```text
             PRD ──────────────┐
              │ satisfies      │ tested_by
   UX Spec ───┤                ▼
              ▼            Test Plan
       Technical Design ◄── depends_on ── ADRs
              │ affects / implements
      ┌───────┴────────┐
   API Spec      Database Design
              │ derived_from
      Implementation Plan ──► code tasks ──► merged code
              │
      Deployment Plan ──► Release Notes          Runbook (operations, ongoing)

   Later changes:  Change Request ──► PRD v2 / Design v2 ──► new plan ──► code
```

### Minimum sets

| Situation | Documents to have |
|-----------|-------------------|
| Small internal change | None. Use the `quick-fix` workflow. |
| New feature | PRD, Technical Design (plus ADRs), Implementation Plan. The `feature` workflow writes these for you. |
| Client project, first release | Architecture Overview, PRD, UX Spec, Technical Design, API Spec, Database Design, Test Plan, Deployment Plan, Runbook, Release Notes |
| Changing something already approved | Change Request, then new versions of the affected documents. The `change` workflow handles this. |

---

## 2. What each document contains

Every template has the sections below. Guidance for each section sits inside `<!-- … -->` comments in the template, and placeholders look like `{{ … }}`. The validator refuses any document that still contains a placeholder or has an empty required section. If a section doesn't apply, write **"Not applicable"** and one sentence explaining why.

| Document | Sections | Based on |
|----------|----------|----------|
| PRD | Summary · Background and Problem Statement · Goals and Success Metrics · Non-Goals · Users and Personas · User Stories · Functional Requirements · Non-Functional Requirements · Assumptions and Constraints · Dependencies · Risks · Release Scope · Open Questions · (Glossary) | Common PRD practice; MoSCoW priorities; Given/When/Then acceptance criteria; ISO/IEC 25010 quality attributes |
| UX Spec | Summary · Users and Scenarios · User Flows · Information Architecture · Screens and States · Interaction and Validation Rules · Content and Copy · Accessibility · Responsive Behaviour · Analytics and Tracking · Open Questions | WCAG 2.2 AA |
| Architecture Overview | Introduction and Goals · Constraints · System Context · Container View · Key Components · Data and Integrations · Deployment View · Cross-Cutting Concerns · Quality Attributes · Architecture Decisions · Risks and Technical Debt · Glossary | arc42, C4 model |
| Technical Design | Purpose · Requirements Addressed · Proposed Solution · Architecture · Data Changes · API Changes · Failure Scenarios · Security Considerations · Observability · Alternatives Considered · Testing Strategy · Rollout and Migration · Open Questions | Design-doc / RFC practice; STRIDE; OWASP |
| ADR | Context and Problem Statement · Decision Drivers · Considered Options · Decision Outcome · Consequences · Pros and Cons of the Options · Confirmation · (More Information) | MADR |
| API Specification | Overview · Conventions · Authentication and Authorization · Endpoints · Data Models · Errors · Versioning and Compatibility · Change Log | OpenAPI 3.1 alongside; RFC 9457 error format; RFC 3339 dates |
| Database Design | Overview · Data Model · Entities · Relationships and Constraints · Indexes and Performance · Migrations and Backfill · Data Privacy and Retention · Backup and Recovery · Open Questions | Data classification; RPO/RTO |
| Test Plan | Introduction and Scope · Test Items · Features to Be Tested · Features Not to Be Tested · Test Approach · Requirements Traceability · Test Cases · Test Environment and Data · Entry and Exit Criteria · Defect Management · Risks and Contingencies · Acceptance and Sign-off | ISO/IEC/IEEE 29119-3 |
| Implementation Plan | Overview · Scope · Work Breakdown (one `yaml` block) · Sequencing and Dependencies · Risks and Mitigations · Definition of Done | — |
| Deployment Plan | Release Summary · Components and Versions · Pre-Deployment Checklist · Deployment Steps · Database Migrations · Configuration Changes · Verification · Rollback Plan · Communication Plan · Post-Deployment Monitoring · Sign-off | Change-management practice (ITIL-style) |
| Runbook | Service Overview · Architecture and Dependencies · Environments and Access · Monitoring and Alerts · Routine Procedures · Troubleshooting · Incident Response and Escalation · Backup and Restore · Contacts | SRE practice |
| Change Request | Summary · Reason for Change · Current Behaviour · Proposed Change · Affected Artifacts · Impact Assessment · Options Considered · Recommendation · Rollback and Contingency | Change control |
| Release Notes | Release Summary · Highlights · Improvements · Bug Fixes · Breaking Changes and Migration · Known Issues · Upgrade Instructions | Semantic Versioning |

Two rules for every client-facing document: **never put secrets in documents** (refer to them by name and location), and **never invent facts**. Unknowns go into Assumptions or Open Questions.

---

## 3. The front matter (the header block)

Every document starts with a YAML header. The platform uses it for identity, lifecycle, and links, and Obsidian shows it as "Properties".

```yaml
---
type: technical-design          # which contract applies
id: TD-001                      # unique within the project
title: "QR Ticket Validation — Technical Design"
project: guest-management       # must match the folder
status: pending_approval        # set by the platform; see lifecycle below
version: 1                      # bumped only through a change request
owner: "Bia, Architect"
created: "2026-09-17"
updated: "2026-09-17"
satisfies: [PRD-001]            # typed links to other documents
depends_on: [ADR-001]
affects: [API-001, DB-001]
---
```

Full rules: [knowledge/contracts/frontmatter.md](../knowledge/contracts/frontmatter.md).

### Relationships

| Key | Meaning | Example |
|-----|---------|---------|
| `satisfies` | This fulfils those requirements | TD-001 satisfies PRD-001 |
| `implements` | This implements that design | API-001 implements TD-001 |
| `depends_on` | This relies on that decision or document | TD-001 depends_on ADR-001 |
| `affects` | Changing this impacts those | TD-001 affects DB-001; CR-001 affects PRD-001 |
| `derived_from` | This was produced from that | PLAN-001 derived_from TD-001; PRD-001 v2 derived_from CR-001 |
| `supersedes` | This replaces that (different) document | ADR-004 supersedes ADR-002 |
| `contradicts` | Known conflict to resolve | ADR-003 contradicts ADR-001 |
| `tested_by` | That test plan verifies this | PRD-001 tested_by TP-001 |
| `documents` | This describes that | RB-001 documents ARCH-001 |
| `references` | Plain "see also" | PRD-001 references GL-001 |

A link can point to documents in the same project, its client, any stack, or global knowledge, but **never to another project or another client**. The platform adds a generated "Relations" section at the bottom of each document so these links show up in Obsidian's graph view. Don't edit that section.

---

## 4. Lifecycle

```text
draft ──submit──► pending_approval ──approve──► approved ──(replaced)──► superseded
                        │
                        ├──request changes──► changes_requested ──(fix, resubmit)──► pending_approval
                        └──reject──► rejected
```

| Status | Meaning | Who sets it |
|--------|---------|-------------|
| `draft` | Being written | Author / platform |
| `pending_approval` | Waiting for your decision | Platform, on submit |
| `approved` | Accepted; now an immutable input to later work | Platform, on your approval |
| `changes_requested` | Sent back with your comment | Platform |
| `rejected` | Not accepted, or withdrawn because its request was cancelled | Platform |
| `superseded` | Replaced by another document | Platform |

**Approved means frozen.** The platform records a fingerprint (content hash) of every approved version. If an approved document is edited, validation fails with *"approved version 1 was modified in place"*. To change it, use a change request (section 6).

**Submitted means frozen too.** If a document is edited after it was submitted, approval is refused until it's submitted again. You always approve exactly what you reviewed.

---

## 5. Writing a document yourself

```powershell
# 1. Create it from the template (gets the next free id, title, owner and dates)
empire docs new -project guest-management -type test-plan -title "Check-in acceptance" -owner "Bia, QA"
#    → knowledge/projects/guest-management/testing/TP-001-check-in-acceptance.md

# 2. Fill it in (VS Code or Obsidian). Replace every {{ … }}; add links in the header.

# 3. Check it
empire docs validate -project guest-management       # full check (control plane running)
empire docs validate -local                          # quick offline check

# 4. Send it for approval, then decide
empire docs submit TP-001 -project guest-management
empire approvals
empire approve 15 -m "OK for UAT"
```

You may approve documents you submitted yourself, because you're the final authority. An AI agent can never approve anything.

## 6. Changing an approved document

Never edit an approved document directly. Instead:

```powershell
empire request create -project guest-management -workflow change `
  -desc "Scanners must work offline for up to 2 hours.`naffects: PRD-001" "Offline scanning"
```

1. The Product Manager agent writes **CR-001** with `affects: [PRD-001]`.
2. Its approval request includes an **impact analysis**: which designs, plans, tasks, and code files are built on PRD-001.
3. After you approve CR-001, the Architect agent writes **PRD-001 version 2** (`derived_from: [CR-001]`), and you approve it.
4. The Planner agent writes a new plan, and the developers implement it.

To write the change request yourself, `empire docs new -type change-request …`, fill in `affects`, and submit it. You can then bump the affected document's `version` by hand, add `derived_from: [CR-00X]`, and submit it too.

Check the impact yourself at any time:

```powershell
empire impact PRD-001 -project guest-management
```

## 7. Handing documents to a client

```powershell
empire docs export -project guest-management -out exports\guest-management
empire docs export -project guest-management -all      # include drafts, marked "Draft"
```

The export contains:

- `README.md`: an index of all documents with type, version, and status.
- One file per **approved** document with:
  - no front matter, guidance comments, or generated blocks;
  - a **Document Control** table under the title: ID, type, version, status, project, owner, dates, who approved it and when, and related documents.

The files are plain Markdown, so they render on GitHub or GitLab and in Notion, Confluence (Markdown import), and most editors. To produce PDF or Word, run them through a converter such as Pandoc.

## 8. Checks the validator runs

| Check | Example message |
|-------|-----------------|
| Front matter present and well-formed | `missing YAML front matter` |
| Known type, id format, unique id | `id "td-1" must look like TD-001` |
| Title, status, version, owner, dates | `` `created: NEW` must be a date written as YYYY-MM-DD `` |
| Right folder and project key | `a Technical Design belongs in projects/shop/architecture/` |
| Required sections present and not empty | `section "Observability" is empty` |
| No leftover placeholders | `unfilled template placeholder {{ Owner }}` |
| Relationship allowed and target exists | `` `affects: PRD-001` must point to api-spec / database-design / … `` |
| No links to other projects or clients | `"projects/other/PRD-001" is outside the scopes this document may reference` |
| Links and wikilinks resolve | `broken link "missing.md"` |
| Upstream approved before approval | `needs at least 1 approved prd in `satisfies` before it can go for approval` |
| Approved versions untouched | `approved version 1 was modified in place` |
| New versions authorised | `version 2 changes an approved document; add derived_from: [CR-…]` |
| Work breakdown is valid | `work breakdown: task 2 (ui): unknown repository "web"` |

`make hooks` installs a git pre-commit hook that runs the offline check whenever you commit changes under `knowledge/`. CI runs it too.

## 9. Glossary

| Term | Meaning |
|------|---------|
| Acceptance criteria | Observable conditions that prove a requirement is met, often "Given … when … then …" |
| ADR | A short record of one significant decision and why it was made |
| arc42 / C4 | Widely used structures for describing software architecture (C4: context → containers → components) |
| MoSCoW | Priority scale: **M**ust, **S**hould, **C**ould, **W**on't (this time) |
| NFR | Non-functional requirement: quality attributes such as performance, security, accessibility |
| RPO / RTO | Recovery Point / Time Objective: how much data you may lose, and how long recovery may take |
| RTM | Requirements Traceability Matrix: requirement → test cases → result |
| UAT | User Acceptance Testing, done by the client before sign-off |
| WCAG | Web Content Accessibility Guidelines |
