# Workflows

A **request** is something you want, written in plain language. A **workflow** turns it into documents, a plan, and reviewed code, and stops wherever you must decide (spec §7).

Definitions live in [workflows/](../workflows/) as YAML, so you can add your own.

```powershell
empire workflows                       # what's available
empire request create -project P [-workflow feature|quick-fix|change] [-repo NAME] -desc "details" Title words
empire request list
empire request get 3                   # steps, tasks, documents, and what's waiting for you
empire approvals                       # everything waiting for you
empire request cancel 3                # stops it, closes its gates, withdraws unapproved documents
```

---

## 1. The three workflows

### `feature`: a new capability

```text
request
  │
  ▼  prd        Product Manager writes PRD-00N          ──► you approve
  ▼  design     Architect writes TD-00N (+ ADRs)        ──► you approve   (TD satisfies the PRD)
  ▼  plan       Planner writes PLAN-00N                 ──► you approve   (PLAN derived from the TD)
  ▼  implement  one code task per work item, in order:
                  Developer codes → tests → AI Reviewer
                    ├─ reviewer asks for changes → Developer again (max 2 rounds)
                    └─ OK → merge gate ──► you approve → merged → next task
  ▼
completed
```

```powershell
empire request create -project guest-management -desc "Guests show a QR code at the door; staff scan it to check them in." "QR ticket validation"
```

### `quick-fix`: a small, well-understood change

No documents. One code task in the repository you name, then AI review, then the merge gate.

```powershell
empire request create -project guest-management -workflow quick-fix -repo frontend "Fix typo on the login page"
```

### `change`: changing already-approved work

```text
change-request   Product Manager writes CR-00N (affects: …)   ──► you approve (with impact analysis)
revise           Architect writes a new version of each affected document, requirements first
                                                                ──► you approve each
plan             Planner writes a new plan                      ──► you approve
implement        code tasks, as in `feature`
```

```powershell
empire request create -project guest-management -workflow change -desc "Scanners must work offline.`naffects: PRD-001" "Offline scanning"
```

If the description names the affected documents, the agent can find them; it can also work them out from the context.

---

## 2. Your decisions

`empire approvals` lists everything waiting for you. Each approval request shows:

- **For documents:** the file path, the agent's summary, any extra documents approved together (for example ADRs with a design), and, for change requests, the impact analysis.
- **For merges:**
  - the branch and commit;
  - the developer's summary;
  - the **AI review** verdict and findings;
  - a diffstat of the changes.

| Command | Documents | Merges |
|---------|-----------|--------|
| `empire approve N [-m "…"]` | Approved; the next step starts | Merged; the next task can start |
| `empire request-changes N -m "…"` | The agent reworks the draft using your comment | The developer reworks the branch using your comment |
| `empire reject N [-m "…"]` | Rejected, and the whole request is cancelled | Cancelled, and the whole request is cancelled |

Open the document named in the request in VS Code or Obsidian before you decide. If a document changes after it was submitted, approval is refused until it's resubmitted.

---

## 3. What the agents receive

Each task gets a minimal context (spec §22):

- **Global knowledge:** engineering and security principles.
- **Its role definition:** [knowledge/roles/](../knowledge/roles/).
- **Stack knowledge:** for code tasks, the repository's stack. Document tasks get all the project's stacks.
- **Client knowledge:** the project's client folder, if the project has a client.
- **Project documents:**
  - the documents from earlier steps;
  - everything they link to: the PRD a design satisfies, the ADRs it depends on, API/DB specs implementing it, and test plans.
- **For document tasks:** the template and contract. Your previous draft or comment is included when reworking.

Document agents write into a scratch folder. The platform validates their output, and any problems go back to the agent (up to 3 attempts) before a human ever sees the document. Agents can't choose ids, versions, dates, or status; the platform sets those.

---

## 4. Writing your own workflow

```yaml
# workflows/api-change.yaml
name: api-change
description: Requirements, design, and an API specification before any code.
steps:
  - name: prd
    kind: document            # document | plan | revise | code
    doc_type: prd
    role: product-manager     # loads knowledge/roles/product-manager.md

  - name: design
    kind: document
    doc_type: technical-design
    role: architect
    inputs: [prd]             # earlier steps whose documents go into the context
    extra_types: [adr]        # other types the step may produce (approved together)
    relations:
      satisfies: [prd]        # links the platform adds automatically

  - name: api
    kind: document
    doc_type: api-spec
    role: architect
    inputs: [prd, design]
    relations:
      implements: [design]

  - name: plan
    kind: plan                # must be followed by a code step
    doc_type: implementation-plan
    role: planner
    inputs: [design, api]
    gate: true                # false = create the code tasks without asking
    relations:
      derived_from: [design]

  - name: implement
    kind: code
    role: developer
    inputs: [design, api, plan]
    review: true              # AI review before the merge gate
```

Rules, checked when the control plane starts:

- Step names are unique.
- `inputs` and `relations` only name earlier steps.
- A `plan` step is directly followed by a `code` step.
- A `revise` step has a change-request step among its inputs.
- `doc_type` values must have a contract.

Contracts still apply at run time. For example, a technical design needs an approved PRD in `satisfies`, and a plan needs an approved design or change request in `derived_from`. That's why this example starts with a `prd` step and adds those links in `relations`.

Document and revise steps always wait for your approval; only a plan step can skip its gate (`gate: false`). After editing workflows, restart the control plane.
