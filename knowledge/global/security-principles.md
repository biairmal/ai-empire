---
type: guideline
id: GL-002
title: Security Principles
status: approved
version: 1
depends_on:
  - GL-001
---

# Security Principles

1. **Least privilege.** Agents and workers get only the access their task needs.
2. **Project isolation.** An agent works only inside its own project's worktree. It never reads unrelated projects.
3. **Secrets isolation.** Secrets are injected per task only when required. They never appear in context bundles, logs, commits, or documents.
4. **No self-granted authority.** AI can't approve its own work, change permissions, or bypass gates.
5. **Protected targets need a human.** Merges to protected branches, production deployments, destructive migrations, and destructive infrastructure operations require explicit human approval.
6. **Authenticated and authorized access.** Every API call is authenticated, and every action is checked against policy.
7. **Audit everything that matters.** State changes, approvals, and privileged actions are logged append-only with the actor recorded.
8. **Global rules are guarded.** Changes to global knowledge or security policy go through human review.
9. **Validate at trust boundaries.** Treat input from agents, external tools, and users as untrusted until validated.

<!-- relations:start — generated from the front matter by the platform; do not edit -->
## Relations

- **Depends on:** [[engineering-principles|GL-001 · Engineering Principles]]
<!-- relations:end -->
