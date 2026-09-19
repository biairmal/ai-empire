// Package api holds the JSON shapes shared by the control plane, worker and CLI.
package api

import "time"

// Client is who the work is for: the confidentiality boundary (spec §11B).
type Client struct {
	ID                   int64     `json:"id" db:"id"`
	Slug                 string    `json:"slug" db:"slug"`
	Name                 string    `json:"name" db:"name"`
	DefaultAutonomyLevel string    `json:"default_autonomy_level" db:"default_autonomy_level"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
}

// Project is a product; its code lives in one or more repositories (spec §11A).
type Project struct {
	ID            int64     `json:"id" db:"id"`
	Slug          string    `json:"slug" db:"slug"`
	Name          string    `json:"name" db:"name"`
	ClientID      *int64    `json:"client_id" db:"client_id"` // nil = personal/internal
	AutonomyLevel string    `json:"autonomy_level" db:"autonomy_level"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type Repository struct {
	ID            int64     `json:"id" db:"id"`
	ProjectID     int64     `json:"project_id" db:"project_id"`
	Name          string    `json:"name" db:"name"`
	RepoURL       string    `json:"repo_url" db:"repo_url"`
	DefaultBranch string    `json:"default_branch" db:"default_branch"`
	Stack         string    `json:"stack" db:"stack"`
	TestCommand   string    `json:"test_command" db:"test_command"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type Task struct {
	ID                   int64     `json:"id" db:"id"`
	ProjectID            int64     `json:"project_id" db:"project_id"`
	RepositoryID         *int64    `json:"repository_id" db:"repository_id"` // code tasks only
	Kind                 string    `json:"kind" db:"kind"`                   // code | document | plan | revise
	Role                 string    `json:"role" db:"role"`
	RequestID            *int64    `json:"request_id" db:"request_id"`
	Step                 string    `json:"step" db:"step"`
	DocType              string    `json:"doc_type" db:"doc_type"`
	Revises              string    `json:"revises" db:"revises"`
	OutputDocs           []string  `json:"output_docs" db:"output_docs"`
	Review               bool      `json:"review" db:"review"`
	ReviewRounds         int       `json:"review_rounds" db:"review_rounds"`
	MergeSHA             string    `json:"merge_sha" db:"merge_sha"`
	ChangedFiles         []string  `json:"changed_files" db:"changed_files"` // files the merge changed
	Title                string    `json:"title" db:"title"`
	Description          string    `json:"description" db:"description"`
	Status               string    `json:"status" db:"status"`
	Stage                string    `json:"stage" db:"stage"`
	RequiredCapabilities []string  `json:"required_capabilities" db:"required_capabilities"`
	ContextDocs          []string  `json:"context_docs" db:"context_docs"`
	Feedback             string    `json:"feedback" db:"feedback"`
	WorkerID             *int64    `json:"worker_id" db:"worker_id"`
	Attempts             int       `json:"attempts" db:"attempts"`
	LastError            string    `json:"last_error" db:"last_error"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`
}

// Request is a plain-language request driven through a workflow (spec §7).
type Request struct {
	ID           int64     `json:"id" db:"id"`
	ProjectID    int64     `json:"project_id" db:"project_id"`
	RepositoryID *int64    `json:"repository_id" db:"repository_id"`
	Workflow     string    `json:"workflow" db:"workflow"`
	Title        string    `json:"title" db:"title"`
	Description  string    `json:"description" db:"description"`
	Status       string    `json:"status" db:"status"`
	CurrentStep  string    `json:"current_step" db:"current_step"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

type Worker struct {
	ID              int64     `json:"id" db:"id"`
	Name            string    `json:"name" db:"name"`
	Capabilities    []string  `json:"capabilities" db:"capabilities"`
	Status          string    `json:"status" db:"status"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at" db:"last_heartbeat_at"`
	TokenHash       *string   `json:"-" db:"token_hash"`
	ProjectIDs      []int64   `json:"project_ids" db:"project_ids"` // empty = any project
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type AgentRun struct {
	ID           int64      `json:"id" db:"id"`
	TaskID       int64      `json:"task_id" db:"task_id"`
	WorkerID     int64      `json:"worker_id" db:"worker_id"`
	Model        string     `json:"model" db:"model"`
	ContextFiles []string   `json:"context_files" db:"context_files"`
	StartedAt    time.Time  `json:"started_at" db:"started_at"`
	FinishedAt   *time.Time `json:"finished_at" db:"finished_at"`
	ExitStatus   *int       `json:"exit_status" db:"exit_status"`
	LogPath      string     `json:"log_path" db:"log_path"`
	Tokens       int64      `json:"tokens" db:"tokens"`
	CostUSD      float64    `json:"cost_usd" db:"cost_usd"`
	Summary      string     `json:"summary" db:"summary"` // what the agent says it did
	Role         string     `json:"role" db:"role"`
	LogTail      string     `json:"log_tail" db:"log_tail"` // end of the agent log, kept on the control plane
}

type ApprovalRequest struct {
	ID             int64     `json:"id" db:"id"`
	ProjectID      *int64    `json:"project_id" db:"project_id"`
	TaskID         *int64    `json:"task_id" db:"task_id"`
	Covers         []string  `json:"covers" db:"covers"` // documents approved together: "<key>@v<version>@<hash>"
	SubjectType    string    `json:"subject_type" db:"subject_type"`
	SubjectRef     string    `json:"subject_ref" db:"subject_ref"`
	SubjectVersion string    `json:"subject_version" db:"subject_version"`
	Gate           string    `json:"gate" db:"gate"`
	Status         string    `json:"status" db:"status"`
	Summary        string    `json:"summary" db:"summary"`
	RequestedBy    string    `json:"requested_by" db:"requested_by"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
}

type AuditEntry struct {
	ID        int64          `json:"id" db:"id"`
	ActorType string         `json:"actor_type" db:"actor_type"`
	ActorID   string         `json:"actor_id" db:"actor_id"`
	Action    string         `json:"action" db:"action"`
	Target    string         `json:"target" db:"target"`
	Payload   map[string]any `json:"payload" db:"payload"`
	CreatedAt time.Time      `json:"created_at" db:"created_at"`
}

// Request bodies.

type CreateProject struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	Client        string `json:"client,omitempty"`         // client slug; empty = personal
	AutonomyLevel string `json:"autonomy_level,omitempty"` // empty = client default, else conservative
}

// Update bodies: nil fields are left unchanged.

type UpdateProject struct {
	Name          *string `json:"name,omitempty"`
	AutonomyLevel *string `json:"autonomy_level,omitempty"`
}

type UpdateRepository struct {
	RepoURL       *string `json:"repo_url,omitempty"`
	DefaultBranch *string `json:"default_branch,omitempty"`
	Stack         *string `json:"stack,omitempty"`
	TestCommand   *string `json:"test_command,omitempty"` // "" clears it
}

type MoveProject struct {
	Client string `json:"client"` // client slug; empty = no client
}

type CreateRepository struct {
	Name          string `json:"name"`
	RepoURL       string `json:"repo_url"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Stack         string `json:"stack"`
	TestCommand   string `json:"test_command,omitempty"`
}

type CreateTask struct {
	Project              string   `json:"project"`              // slug or id
	Repository           string   `json:"repository,omitempty"` // name; optional if the project has one repo
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	RequiredCapabilities []string `json:"required_capabilities"`
	ContextDocs          []string `json:"context_docs"`
	DependsOn            []int64  `json:"depends_on"`
}

type TaskDetail struct {
	Task      Task              `json:"task"`
	Runs      []AgentRun        `json:"runs"`
	Approvals []ApprovalRequest `json:"approvals"`
}

type RegisterWorker struct {
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

type Heartbeat struct {
	TaskID int64 `json:"task_id,omitempty"`
}

type HeartbeatReply struct {
	Cancel bool `json:"cancel"` // the held task is no longer this worker's to run
}

type Claim struct {
	Task       Task       `json:"task"`
	Project    Project    `json:"project"`
	Repository Repository `json:"repository"`
}

type Transition struct {
	To           string   `json:"to"`
	Error        string   `json:"error,omitempty"`
	MergeSHA     string   `json:"merge_sha,omitempty"`     // with to=COMPLETED: the commit on the default branch
	ChangedFiles []string `json:"changed_files,omitempty"` // with to=COMPLETED: files the merge changed
}

type Authorize struct {
	Action         string `json:"action"`
	Stage          string `json:"stage"`           // where to resume once approved
	Summary        string `json:"summary"`         // shown to the human
	SubjectVersion string `json:"subject_version"` // e.g. branch commit sha
}

type AuthorizeReply struct {
	Allowed        bool   `json:"allowed"`
	ApprovalID     int64  `json:"approval_id,omitempty"`
	SubjectVersion string `json:"subject_version,omitempty"` // what the human approved
}

type StartRun struct {
	Model        string   `json:"model"`
	ContextFiles []string `json:"context_files"`
	Role         string   `json:"role"`
}

type FinishRun struct {
	ExitStatus int     `json:"exit_status"`
	LogPath    string  `json:"log_path"`
	Tokens     int64   `json:"tokens"`
	CostUSD    float64 `json:"cost_usd"`
	Summary    string  `json:"summary"`
	LogTail    string  `json:"log_tail"`
}

type Decision struct {
	Comment     string `json:"comment"`
	ConfirmCode string `json:"confirm_code,omitempty"` // required when Hermes relays the decision
}

type ID struct {
	ID int64 `json:"id"`
}

// V3: workflows and documents.

type CreateRequest struct {
	Project     string `json:"project"`
	Workflow    string `json:"workflow"`             // default "feature"
	Repository  string `json:"repository,omitempty"` // required by workflows that start with code
	Title       string `json:"title"`
	Description string `json:"description"`
}

type RequestDetail struct {
	Request   Request           `json:"request"`
	Steps     []StepStatus      `json:"steps"`
	Tasks     []Task            `json:"tasks"`
	Documents []DocumentInfo    `json:"documents"`
	Approvals []ApprovalRequest `json:"approvals"`
}

type StepStatus struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Role   string `json:"role"`
	Status string `json:"status"` // not_started | in_progress | waiting_for_human | completed | cancelled
}

type DocumentInfo struct {
	Key      string `json:"key"`
	Scope    string `json:"scope"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Path     string `json:"path"`
	Status   string `json:"status"`
	Version  int    `json:"version"`
	Approved bool   `json:"approved"` // current content is an approved version
}

type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

// TaskContext is everything a worker needs to run a task's agent.
type TaskContext struct {
	Files        []string            `json:"files"`   // knowledge files in the bundle
	Content      string              `json:"content"` // the bundle itself
	Kind         string              `json:"kind"`
	Role         string              `json:"role"`
	DocType      string              `json:"doc_type,omitempty"`
	Templates    map[string]string   `json:"templates,omitempty"` // doc type → template
	Contracts    map[string]string   `json:"contracts,omitempty"` // doc type → contract YAML
	Request      *Request            `json:"request,omitempty"`
	Repositories []Repository        `json:"repositories,omitempty"`
	Drafts       []File              `json:"drafts,omitempty"`    // current versions to rework or revise
	Relations    map[string][]string `json:"relations,omitempty"` // relationships the platform will add
}

type TaskOutput struct {
	Files   []File `json:"files"`
	Summary string `json:"summary"`
}

type OutputReply struct {
	Accepted  bool     `json:"accepted"`
	Problems  []string `json:"problems,omitempty"`
	Documents []string `json:"documents,omitempty"`
	Status    string   `json:"status,omitempty"` // task status after acceptance
}

type ReviewReport struct {
	Verdict string `json:"verdict"` // APPROVE | REQUEST_CHANGES | NONE
	Summary string `json:"summary"`
}

type ReviewReply struct {
	Rework bool `json:"rework"` // the task went back to the developer
}

type NewDocument struct {
	Project string `json:"project"`
	Type    string `json:"type"`
	Title   string `json:"title"`
	Owner   string `json:"owner"`
}

type DocumentRef struct {
	Ref     string `json:"ref"`               // knowledge-relative path, or key "projects/x/PRD-001"
	Project string `json:"project,omitempty"` // lets Ref be a bare id
}

type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

type TraceNode struct {
	Kind     string      `json:"kind"` // request | document | task | commit
	Ref      string      `json:"ref"`
	Title    string      `json:"title"`
	Status   string      `json:"status,omitempty"`
	Via      string      `json:"via,omitempty"` // relationship that led here
	Children []TraceNode `json:"children,omitempty"`
}

type Trace struct {
	Subject TraceNode   `json:"subject"`
	Why     []TraceNode `json:"why"`     // upstream: what this exists for
	Effects []TraceNode `json:"effects"` // downstream: what depends on this
}

type Impact struct {
	Document  DocumentInfo  `json:"document"`
	Documents []ImpactedDoc `json:"documents"`
	Tasks     []Task        `json:"tasks"`
	Report    string        `json:"report"`
}

type ImpactedDoc struct {
	DocumentInfo
	Via   string `json:"via"`
	Depth int    `json:"depth"`
}

// V2: remote workers.

type CreateWorker struct {
	Name     string   `json:"name"`
	Projects []string `json:"projects,omitempty"` // slugs or ids the worker may serve; empty = any
}

type WorkerToken struct {
	Worker Worker `json:"worker"`
	Token  string `json:"token"` // shown once
}
