# AI Software Development Empire — Master Requirements

## 1. Vision

Build a self-hosted, autonomous AI software development platform that acts as a personal software engineering organization.

The system should allow the owner to manage multiple freelance, client, and personal software projects from a laptop, phone, web interface, or conversational AI interface.

The owner communicates primarily in natural human language.

For example:

> "I want to add QR-based ticket validation to the event platform."

The platform should transform that request into structured engineering work, determine the appropriate workflow, generate required engineering artifacts, request human approval at defined decision boundaries, and then allow AI workers to autonomously execute authorized work.

The owner remains the final decision-maker.

The goal is:

> **Autonomous execution with controlled authority.**

AI should not require human approval for every low-level operation, but it must stop at predefined decision boundaries where human judgment or authorization is required.

---

# 2. Core Principles

The platform must follow these principles:

1. **Human-controlled authority**
2. **Autonomous execution**
3. **Explicit approval gates**
4. **Project isolation**
5. **Technology-stack isolation**
6. **Versioned engineering knowledge**
7. **Minimal context loading**
8. **Explicit relationships between knowledge artifacts**
9. **Auditable AI actions**
10. **Replaceable AI models**
11. **Least privilege**
12. **Simple over clever**
13. **Explicit over magical**
14. **Versioned over implicit**
15. **Isolated over shared**
16. **Reusable knowledge over repeated prompting**
17. **Avoid unnecessary infrastructure complexity**

---

# 3. High-Level Architecture

```text
                         HUMAN
                    Laptop / Phone
                          │
                 Natural Language
                          │
                          ▼
                       HERMES
                AI Interface / Operator
                          │
                          ▼
                 ┌──────────────────┐
                 │    AI DEV OS     │
                 │   CONTROL PLANE  │
                 └────────┬─────────┘
                          │
             ┌────────────┼─────────────┐
             │            │             │
          Workflow      Approval     Scheduler
             │            │             │
             │            │             ▼
             │            │        ┌──────────┐
             │            │        │ WORKERS  │
             │            │        └────┬─────┘
             │            │             │
             │            │       Isolated Workspace
             │            │             │
             │            │             ▼
             │            │        Coding Agent
             │            │             │
             │            │       Code / Tests
             │            │             │
             └────────────┴─────────────┘
                          │
                          ▼
                       Git
                          │
                    Knowledge Graph
                          │
                    Markdown + Git
                          │
                       Obsidian
```

---

# 4. Human Interface

The owner should be able to interact with the platform using natural language.

Examples:

> "Build QR ticket validation."

> "Continue the event ticketing project."

> "What is currently running?"

> "Review the architecture for the payment module."

> "Approve the current PRD."

> "Why did the worker choose this database design?"

> "Show me what will be affected if we change this requirement."

The interface should hide unnecessary implementation details from the human.

The owner should not need to manually assign workers, create queues, construct prompts, or manage agent processes.

---

# 5. Hermes

Hermes acts as the persistent AI interface/operator.

Responsibilities may include:

* natural-language interaction
* understanding user requests
* querying the AI Dev OS
* creating requests/tasks through controlled APIs
* reporting workflow status
* presenting approval requests
* explaining decisions
* notifying the owner of failures or required decisions
* monitoring long-running work

Hermes should **not** be the authoritative source of project/task state.

Hermes should communicate with the AI Dev OS through a controlled API/MCP interface.

Hermes should not directly modify the database.

---

# 6. AI Dev OS / Control Plane

The Control Plane is the authoritative orchestration system.

It owns:

* projects
* project configuration
* workflows
* tasks
* task dependencies
* approval gates
* approval requests
* human decisions
* workers
* worker capabilities
* agent runs
* context resolution
* policies
* permissions
* audit records
* execution state
* knowledge indexing

The initial implementation should preferably be a **modular monolith**, not a large microservice architecture.

---

# 7. Natural-Language Request → Engineering Workflow

A request should follow a controlled lifecycle.

Example:

```text
Human Request
      ↓
Request Understanding
      ↓
Determine Project
      ↓
Determine Workflow
      ↓
Generate PRD
      ↓
PRD Approval Gate
      ↓
Technical Architecture
      ↓
Architecture Approval Gate
      ↓
Implementation Plan
      ↓
Optional Plan Approval
      ↓
Create Tasks
      ↓
AI Workers Execute
      ↓
Automated Testing
      ↓
AI Review
      ↓
Human Merge Approval
      ↓
Deployment
      ↓
Production Approval if required
```

Different workflows can have different gates.

---

# 8. Approval Gates

Approval gates are first-class objects in the platform.

The system must support at least:

```text
PENDING_APPROVAL
APPROVED
CHANGES_REQUESTED
REJECTED
```

Human actions:

* Approve
* Request changes
* Reject

Approval gates can exist for:

* Product Requirements Documents
* UX/UI specifications
* Technical architecture
* Architecture decisions
* Implementation plans
* Significant API changes
* Database changes
* Security-sensitive changes
* Merge
* Production deployment
* Destructive infrastructure operations
* Knowledge promotion
* Permission changes

Not every task requires a gate.

---

# 9. Authority Model

The fundamental rule is:

> **AI may execute authorized work but cannot grant itself additional authority.**

AI may:

* analyze
* research
* plan
* create documents
* write code
* modify its worktree
* run tests
* run static analysis
* create commits
* perform authorized infrastructure operations
* perform authorized deployments

AI may not:

* bypass approval gates
* approve its own work
* silently modify approved requirements
* silently modify approved architecture
* grant itself permissions
* access unrelated projects
* promote project-specific knowledge automatically
* deploy to protected production environments without authorization
* modify global engineering rules without authorization

---

# 10. Risk-Based Autonomy

Actions should have different authority levels.

### Low-risk

Automatically allowed:

* read project source
* create branch/worktree
* modify code
* run tests
* format code
* static analysis
* generate documentation
* refactor code
* create commits

### Medium-risk

Subject to project policy:

* database schema changes
* API contract changes
* dependency additions
* authentication changes
* infrastructure configuration
* architectural changes

### High-risk

Require explicit human approval:

* merge to protected branch
* production deployment
* destructive migration
* destructive infrastructure operation
* security policy changes
* global knowledge promotion
* permission escalation

---

# 11. Project Autonomy Policies

Different projects may have different autonomy levels.

Example:

```text
Personal project
    High autonomy

Internal project
    Medium autonomy

Client project
    Conservative autonomy
```

Policies determine which operations require approval.

Approval behavior should therefore be configurable rather than hardcoded into workers.

---

# 12. Approved Artifacts Must Be Versioned

Important engineering artifacts must be versioned.

Example:

```text
PRD-001 v1
      ↓
APPROVED
      ↓
Architecture v1
      ↓
APPROVED
```

Once approved, an artifact version becomes an immutable input to downstream work.

If an agent discovers that an approved requirement or architecture needs to change:

```text
Approved PRD v1
      ↓
Change discovered
      ↓
Change Request
      ↓
PRD v2
      ↓
Human Approval
```

The AI must not silently modify the approved artifact.

---

# 13. Engineering Knowledge System

Engineering knowledge should be stored as:

> **Markdown files managed by Git.**

The knowledge repository is versioned infrastructure.

It should not be a single giant documentation file.

Documentation must be separated by concern.

Example:

```text
project/
├── requirements/
├── ux/
├── architecture/
├── decisions/
├── api/
├── database/
├── testing/
└── operations/
```

---

# 14. Knowledge Graph

Documents should be connected through explicit relationships.

Example:

```text
PRD
 │
 ├── satisfies → Requirement
 │
 ▼
Technical Design
 │
 ├── affects → API
 ├── affects → Database
 ├── affects → UI
 └── depends_on → ADR
```

The goal is a **spider-web-like engineering knowledge graph**.

A human or AI should be able to navigate:

```text
Requirement
   ↓
PRD
   ↓
Architecture
   ↓
API
   ↓
Database
   ↓
Implementation
   ↓
Tests
```

And backwards:

```text
Code
 ↓
Task
 ↓
Technical Design
 ↓
Requirement
 ↓
Original Request
```

---

# 15. Typed Document Relationships

Relationships should have semantic meaning.

Examples:

```text
satisfies
implements
depends_on
affects
derived_from
supersedes
contradicts
tested_by
documents
references
```

Documents should not merely contain arbitrary links.

Example metadata:

```yaml
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
```

---

# 16. Document Contracts

Every document type must have a defined contract.

A contract specifies:

* purpose
* required metadata
* required sections
* allowed relationships
* lifecycle
* validation rules
* approval requirements
* versioning rules

Examples:

```text
PRD
Technical Design
ADR
UX Specification
API Specification
Database Design
Test Plan
Deployment Plan
Research Document
Architecture Overview
```

---

# 17. Document Templates

Each document type must have a standard template.

For example, a Technical Design template may require:

```text
1. Purpose
2. Requirements addressed
3. Proposed solution
4. Architecture
5. Data changes
6. API changes
7. Failure scenarios
8. Security considerations
9. Alternatives considered
10. Testing strategy
11. Open questions
```

This ensures that humans and AI agents produce consistent artifacts.

---

# 18. Document Validation

Generated documentation must be validated before entering the workflow.

Validation should check:

* required sections
* valid metadata
* unique document IDs
* valid project
* valid relationships
* referenced documents exist
* lifecycle state is valid
* version is valid
* no broken links
* required upstream artifacts exist
* required upstream artifacts are approved

Invalid documents should return to the generating agent for correction.

---

# 19. Document Contracts by Concern

Each document type should answer a different question.

### PRD

**What problem are we solving and why?**

### UX/UI

**How should the user experience the feature?**

### Technical Design

**How will the system technically satisfy the requirements?**

### ADR

**Why did we choose this solution?**

### API Specification

**What contract does the system expose?**

### Database Design

**How is the data represented and constrained?**

### Test Plan

**How do we prove the requirements are satisfied?**

This prevents duplication and giant documentation files.

---

# 20. Obsidian

Obsidian should be supported as a knowledge-graph interface over the Markdown repository.

Obsidian is not the authoritative database.

The authoritative knowledge source is:

```text
Markdown + Git
```

Obsidian provides:

* graph visualization
* backlinks
* document navigation
* local knowledge exploration
* human-friendly editing

The underlying Markdown repository must remain usable without Obsidian.

---

# 21. Knowledge Hierarchy

Knowledge must be isolated by scope.

```text
GLOBAL ENGINEERING KNOWLEDGE
          ↓
TECH STACK KNOWLEDGE
          ↓
PROJECT KNOWLEDGE
          ↓
FEATURE / TASK CONTEXT
```

Information flows downward.

It must not automatically flow sideways.

Example:

```text
Global
 ├── Engineering principles
 └── Security principles

Go
 ├── Go architecture
 ├── Go testing
 └── Go libraries

.NET
 ├── .NET architecture
 ├── EF Core
 └── .NET testing

Project A
 └── Client-specific knowledge

Project B
 └── Different client-specific knowledge
```

Project A must not automatically leak knowledge into Project B.

Go knowledge must not automatically appear in .NET tasks.

---

# 22. Context Resolver

AI agents must not receive the entire knowledge repository.

The Control Plane should dynamically construct a minimal context bundle.

Example:

```text
Task
 ↓
Relevant PRD
 ↓
Relevant Technical Design
 ↓
Relevant API
 ↓
Relevant Database Design
 ↓
Relevant Tests
 ↓
Relevant Stack Rules
 ↓
Relevant Project Rules
```

The context resolver should exclude irrelevant information.

Goals:

* lower token consumption
* reduce hallucination
* reduce context pollution
* improve reasoning
* improve execution speed
* maintain project isolation

---

# 23. Learning System

The platform should eventually support controlled organizational learning.

Flow:

```text
Project Experience
       ↓
Learning Candidate
       ↓
Human Review
       ↓
Approved
       ↓
Global or Stack Knowledge
```

AI must not automatically turn client-specific knowledge into global knowledge.

For example:

```text
Client A discovered a useful Go pattern
             ↓
Learning Candidate
             ↓
Human reviews
             ↓
Generalizable?
        ┌────┴────┐
       NO         YES
        │           │
Project-only     Go Knowledge
```

---

# 24. Git

Git should be the version-control mechanism for engineering knowledge and source code.

Knowledge Git should provide:

* history
* versioning
* diffs
* rollback
* branching
* review
* auditability

Important knowledge changes should be reviewable.

---

# 25. Backups

Knowledge repositories must be backed up independently.

At minimum:

```text
Primary Git Repository
        │
        ├── Remote Backup
        ├── Scheduled Backup
        └── Offline/Secondary Backup
```

Backups should be:

* automated
* scheduled
* retained according to policy
* protected
* periodically tested for restoration

A backup should not be considered reliable until restoration has been tested.

Operational databases such as PostgreSQL require their own backup strategy.

---

# 26. AI Agent Roles

The system should support specialized agent roles.

Initial roles:

```text
Architect
Planner
Developer
Tester
Reviewer
Researcher
Documentation
DevOps
```

Each role should have:

* defined responsibilities
* allowed tools
* allowed actions
* required context
* output contracts
* authority limits

The agent that produces an artifact must not automatically approve that artifact.

---

# 27. Workers

Workers are execution environments.

Mental model:

```text
Machine
   ↓
Worker
   ↓
Task
   ↓
Workspace
   ↓
AI Agent
   ↓
Code / Tests / Git
```

Worker responsibilities:

1. Register
2. Report capabilities
3. Wait for work
4. Claim task
5. Prepare workspace
6. Load context
7. Start AI agent
8. Monitor execution
9. Run validation
10. Report result
11. Clean up

---

# 28. Worker Capabilities

Workers advertise capabilities.

Example:

```text
worker: mac-mini-01

capabilities:
  - go
  - dotnet
  - node
  - docker

resources:
  cpu
  memory
  disk
```

The scheduler should assign tasks based on required capabilities.

---

# 29. Workspace Isolation

Each task should execute in an isolated workspace.

Preferred model:

```text
Git Repository
      ↓
Git Worktree
      ↓
Disposable Development Environment
      ↓
AI Agent
```

Docker should be used where appropriate.

Multiple workers must be able to operate concurrently without interfering with each other.

Agents should not have unrestricted access to unrelated project files.

---

# 30. Task Lifecycle

Tasks should have explicit states:

```text
PENDING
ASSIGNED
RUNNING
TESTING
REVIEWING
WAITING_FOR_HUMAN
COMPLETED
FAILED
CANCELLED
```

The system must survive:

* worker crashes
* agent crashes
* network failures
* Control Plane restarts
* machine reboots

Workers should send heartbeats.

Stale workers/tasks should be detected and recovered/requeued according to policy.

---

# 31. Source of Truth

Different systems have different responsibilities.

### PostgreSQL

Source of truth for:

```text
projects
tasks
workflows
workers
agent runs
approval gates
approval decisions
permissions
audit logs
execution state
```

### Redis

Operational coordination:

```text
queues
streams
temporary state
worker coordination
locks where necessary
```

### Git

Source of truth for:

```text
source code
PRDs
architecture
ADRs
API specifications
database designs
UX/UI specifications
testing documentation
engineering knowledge
templates
contracts
project documentation
```

### Hermes Memory

Only Hermes-specific operational/personal knowledge.

It should not become the authoritative project/task database.

---

# 32. Model Abstraction

The system must not depend permanently on one AI provider.

It should support replaceable models such as:

```text
Claude
GPT
GLM
Local Models
Future Providers
```

Model selection can eventually consider:

* task complexity
* capability
* cost
* speed
* privacy
* availability
* context requirements

---

# 33. MCP / Platform API

Hermes should interact with the Control Plane through a controlled interface.

Example operations:

```text
list_projects
get_project
create_project

list_tasks
get_task
create_task
start_task
cancel_task
retry_task

list_workers
get_worker_status

get_workflow_status
get_agent_run

list_approval_requests
get_approval_request
approve
request_changes
reject
```

Hermes should not directly manipulate PostgreSQL.

---

# 34. Security

Security must be designed around least privilege.

The platform must enforce:

* project isolation
* workspace isolation
* worker permissions
* agent permissions
* filesystem boundaries
* secrets isolation
* API authentication
* authorization
* audit logging
* production restrictions

A developer agent should generally be able to:

```text
Read project source
Modify its worktree
Run tests
Create commits
```

It should not automatically be able to:

```text
Read unrelated projects
Read unnecessary secrets
Modify global knowledge
Deploy production
Change platform security policies
```

---

# 35. Human-in-the-Loop

The system is autonomous but human-controlled.

Human approval should be required for significant decisions such as:

```text
PRD approval
Architecture approval
Significant requirement changes
Significant architectural changes
Sensitive security changes
Protected merges
Production deployment
Destructive infrastructure operations
Global knowledge promotion
Permission escalation
```

Routine low-risk implementation should remain autonomous.

---

# 36. Observability

The owner should be able to see:

```text
Projects
Tasks
Workflows
Workers
Agents
Current activity
Failures
Retries
Execution duration
AI model usage
Token usage
Cost
Git changes
Test results
Review results
Approval requests
```

Example query:

> "What is currently running?"

The system should provide a concise operational summary.

---

# 37. Change Impact Analysis

Because documentation forms a graph, the system should eventually support impact analysis.

Example:

```text
PRD changed
   ↓
Find related artifacts
   ↓
Technical Design
API
Database
UI
Tests
Tasks
Code
```

The system should be able to tell the owner:

> "This requirement change may affect these artifacts."

This should happen **before** autonomous implementation of significant changes.

---

# 38. Traceability

The platform should maintain traceability from:

```text
Human Request
      ↓
PRD
      ↓
Architecture
      ↓
Technical Design
      ↓
Task
      ↓
Code
      ↓
Tests
```

And backwards:

```text
Code
 ↓
Task
 ↓
Technical Design
 ↓
Requirement
 ↓
Human Request
```

The owner should be able to ask:

> "Why does this code exist?"

and:

> "What code is affected by this requirement?"

---

# 39. Notifications

The system should notify the owner when:

* approval is required
* a task fails
* a worker becomes unavailable
* a workflow is blocked
* an agent needs clarification
* tests fail repeatedly
* an important architectural conflict is discovered
* production deployment is ready
* significant work is completed

Notifications should work through the chosen interface such as Hermes/messaging/web.

---

# 40. What the System Should NOT Become

Avoid premature complexity.

Do not start with:

* Kubernetes
* dozens of microservices
* massive multi-agent frameworks
* unnecessary vector databases
* Kafka without a demonstrated requirement
* complex distributed workflow infrastructure
* unrestricted autonomous agents
* automatic global memory mutation
* excessive abstractions

Start with:

```text
Go Control Plane
+
PostgreSQL
+
Redis
+
Git
+
Markdown Knowledge Repository
+
One Worker
+
One Coding Agent
```

Then evolve the system based on real requirements.

---

# 41. Initial Implementation Phases

## V1 — Core Platform

Implement:

```text
Go Control Plane
PostgreSQL
Redis
Git integration
Project isolation
Task management
Basic workflow engine
Approval gates
Context loading
One worker
One AI coding agent
```

Goal:

> Human creates a task → worker autonomously executes it under controlled authority.

---

## V2 — Persistent AI Interface

Add:

```text
Hermes
MCP/API integration
Remote worker
Mac Mini / Mini PC worker
Natural-language task creation
Notifications
```

Goal:

> Manage development from phone/laptop without manually operating the infrastructure.

---

## V3 — AI Software Organization

Add:

```text
Architect
Planner
Developer
Tester
Reviewer
Documentation
DevOps
```

Add:

```text
Workflow orchestration
Document contracts
Templates
Knowledge graph
Obsidian integration
Change impact analysis
```

Goal:

> AI can perform the entire software development lifecycle while stopping at defined human decision gates.

---

## V4 — Autonomous Software Factory

Add:

```text
Multiple workers
Multiple AI models
Model routing
Cost optimization
Advanced scheduling
Learning system
Advanced observability
Automated knowledge indexing
Advanced policy engine
```

Goal:

> Multiple projects can run concurrently with minimal human intervention while maintaining strict project, technology, security, and authority boundaries.

---

# 42. Final Target Experience

The final user experience should feel like this:

### Human

> "I want to add QR-based ticket validation to the event platform."

### System

> I created a PRD based on your request.
> Review required.

### Human

> "Approved."

### System

> Technical architecture is ready.
> It proposes local QR decoding, server-side token validation, transactional duplicate-scan prevention, and scan history persistence.
> Review required.

### Human

> "Approved."

### System

> Implementation plan created. 8 tasks are ready.

The scheduler automatically assigns them.

```text
Worker 1 → API
Worker 2 → Database
Worker 3 → Tests
Worker 4 → UI
```

The workers execute independently.

```text
Code
 ↓
Tests
 ↓
Review
```

The system eventually reports:

> Implementation completed. All automated tests pass. AI review found no blocking issues. Merge approval required.

### Human

> "Approved."

The system merges.

If production deployment requires approval:

> Production deployment is ready. Approval required.

The human decides.

---

# 43. One-Sentence Definition

> **Build a self-hosted AI software factory where humans communicate naturally through Hermes or other interfaces, the AI Dev OS manages projects, workflows, policies, knowledge, tasks, approvals, and workers, isolated AI workers autonomously execute authorized engineering work, and a versioned Markdown knowledge graph connects requirements, UX, architecture, APIs, databases, tests, and implementation without allowing project or technology-specific knowledge to leak across boundaries.**

# 44. Fundamental Mental Model

The entire system can ultimately be understood as:

```text
HUMAN
  │
  │ intent
  ▼
HERMES
  │
  │ controlled commands
  ▼
AI DEV OS
  │
  ├── Workflow
  ├── Policy
  ├── Approval
  ├── Context
  ├── Scheduler
  └── Audit
  │
  ▼
WORKERS
  │
  ▼
AI AGENTS
  │
  ▼
CODE / TESTS / DOCUMENTATION
  │
  ▼
GIT
  │
  ▼
KNOWLEDGE GRAPH
```

**The human controls authority.
The AI controls execution.
The Dev OS controls coordination.
Git controls knowledge history.
The knowledge graph controls traceability.**
