---
type: contract
id: CON-000
status: approved
version: 1
---

# Frontmatter Contract (all documents)

Every Markdown document in `knowledge/` starts with YAML frontmatter. Per-type contracts (`contracts/<type>.yaml`) add to this one: required sections, extra required keys, allowed relationships, upstream approvals, and whether approval is required. The validator (`empire docs validate`) enforces all of it.

## Required keys

| Key | Type | Rule |
|-----|------|------|
| `type` | string | A type with a contract in this folder: `prd`, `ux-spec`, `architecture-overview`, `technical-design`, `adr`, `api-spec`, `database-design`, `test-plan`, `implementation-plan`, `deployment-plan`, `runbook`, `change-request`, `release-notes`, `guideline`, `role` |
| `id` | string | Unique **within its scope** (each project has its own `PRD-001`). Format `<PREFIX>-<NNN>` with the prefix from the contract, e.g. `PRD-001`. New documents written by agents use `NEW`; the platform assigns the number |
| `title` | string | Human-readable title |
| `status` | enum | See lifecycle below |
| `version` | int | Starts at `1`. Bumped only through an approved change request once a version is `approved` (spec §12) |

Most project document types also require `owner`. Optional dates `created`, `updated`, `decision_date`, `release_date` must be written `YYYY-MM-DD`.

## Scope keys

The scope comes from the folder the document lives in, and the frontmatter must match it:

| Folder | Key |
|--------|-----|
| `global/`, `roles/` | none |
| `stacks/<stack>/` | `stack: <stack>` |
| `clients/<slug>/` | `client: <slug>` |
| `projects/<slug>/` | `project: <slug>` |

A document may only reference documents in its own scope or a scope above it, never a sibling scope (spec §11B, §21):

- a project doc may reference its own client, any stack, and global
- a client doc may reference any stack and global
- a stack doc may reference global
- no doc may reference another client or another project

A reference is either an id (`PRD-001`), looked up in the most specific visible scope first (own → client → stacks → global), or a qualified key such as `global/GL-001` or `stacks/go/ADR-002`. When the same id exists in several stacks, qualify it.

## Lifecycle (`status`)

```text
draft → pending_approval → approved → superseded
              │
              ├→ changes_requested → draft
              └→ rejected
```

An `approved` version is immutable. To change it, create a new version that `supersedes` it.

## Relationship keys (optional)

Each key takes a list of document IDs. Only these keys are allowed (spec §15):

| Key | Meaning |
|-----|---------|
| `satisfies` | This doc fulfills the listed requirement/PRD |
| `implements` | This doc/code implements the listed design |
| `depends_on` | This doc relies on the listed doc (often an ADR) |
| `affects` | A change here impacts the listed doc |
| `derived_from` | This doc was produced from the listed doc |
| `supersedes` | This version replaces the listed doc/version |
| `contradicts` | Known conflict with the listed doc; must be resolved |
| `tested_by` | The listed test plan verifies this doc |
| `documents` | This doc describes the listed artifact |
| `references` | Plain informational link |

## Example

```yaml
---
type: technical-design
id: TD-001
project: event-platform
status: approved
version: 2
satisfies:
  - PRD-001
depends_on:
  - ADR-001
affects:
  - API-001
  - DB-001
---
```
