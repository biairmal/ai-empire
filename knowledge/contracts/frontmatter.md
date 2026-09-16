---
type: contract
id: CON-000
status: approved
version: 1
---

# Frontmatter Contract (all documents)

Every Markdown document in `knowledge/` starts with YAML frontmatter. Per-type contracts (M3.1) add to this one; they never loosen it.

## Required keys

| Key | Type | Rule |
|-----|------|------|
| `type` | string | A known document type, e.g. `prd`, `technical-design`, `adr`, `api-spec`, `database-design`, `test-plan`, `guideline`, `contract`, `template` |
| `id` | string | Unique across the whole knowledge repo. Format `<PREFIX>-<NNN>`, e.g. `PRD-001` |
| `status` | enum | See lifecycle below |
| `version` | int | Starts at `1`. Bumped only through a change request once a version is `approved` (spec §12) |

## Scope keys

The scope comes from the folder the document lives in, and the frontmatter must match it:

| Folder | Key |
|--------|-----|
| `global/` | none |
| `stacks/<stack>/` | `stack: <stack>` |
| `projects/<slug>/` | `project: <slug>` |

A document may only reference documents in its own scope or a scope above it (project → stack → global), never a sibling scope (spec §21).

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
