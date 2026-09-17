# Agent Roles

One file per role (spec §26). The file name is the role name used by workflows (`role: architect`).
Each role file is loaded into the context of every task performed in that role.

| Role | Produces | Used in workflow step |
|------|----------|-----------------------|
| [product-manager](product-manager.md) | PRD, Change Request | `prd`, `change-request` |
| [architect](architect.md) | Technical Design, ADRs, revised documents | `design`, `revise` |
| [planner](planner.md) | Implementation Plan (→ tasks) | `plan` |
| [developer](developer.md) | Code and tests | `implement` |
| [reviewer](reviewer.md) | Review verdict before the human merge gate | `implement` (review) |

Tester, Documentation, and DevOps roles are added when a workflow step needs them.

Role files are type `role` and require approval like any other governing document: change them with `empire docs submit`.
