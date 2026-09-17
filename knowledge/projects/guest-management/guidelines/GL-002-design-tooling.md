---
type: guideline
id: GL-002
title: "Design system and design tooling"
project: guest-management
status: approved
version: 1
owner: "Bandana Irmal A, Product Owner"
created: "2026-09-17"
updated: "2026-09-17"
depends_on: []
references: [UX-001]
---

# Design system and design tooling

## Applies To

UX Designer tasks, and developer tasks in the `frontend` repository.

## Tools

| Tool | Purpose | Location |
|------|---------|----------|
| Design system | Light and dark tokens (Direction D "Full Dark Elevated"), app shell, breadcrumb, filter and sort toolbar, core components | `frontend/docs/DESIGN_SYSTEM.md` (summarised in UX-001) |
| Claude Design canvas (current) | Visual mock-ups, one page per area, with a light and a dark version of each | Link in `frontend/docs/design-canvas.local.md` (git-ignored, owner only) |
| Figma (possible future replacement) | Same purpose | Not set up yet |
| Ant Design 6 | Component library; check that a component exists in v6 before naming it | `frontend/package.json` |

## Rules

1. Reuse the tokens and patterns in `DESIGN_SYSTEM.md`. A new token is added there for both themes, never as a one-off colour.
2. Every screen is specified for both light and dark themes, with all states, and with its behaviour on narrow screens.
3. List screens are for viewing and navigating only. Changes happen on detail or create pages, and each account-level action has its own confirmation dialog (UX-001).
4. The UX specification is the source of truth for behaviour. Visual mock-ups are linked, not copied into the specification. Canvas and Figma links are private: write "see the design canvas for this area" instead of the URL.
5. Reader notes (rationale) never go inside a rendered mock-up.
6. Name screens after real Ant Design components.

## How to Run

Not applicable: design work produces documents and links. Frontend: `bun dev` (Next.js 16; read `node_modules/next/dist/docs/` before writing code, as the frontend's `AGENTS.md` requires).

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **References:** [[UX-001-login-and-user-management|UX-001 · Login and User Management — UX Specification]]
<!-- relations:end -->
