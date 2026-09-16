// Package api holds the JSON shapes shared by the control plane, worker and CLI.
package api

import "time"

type Project struct {
	ID            int64     `json:"id" db:"id"`
	Slug          string    `json:"slug" db:"slug"`
	Name          string    `json:"name" db:"name"`
	RepoURL       string    `json:"repo_url" db:"repo_url"`
	DefaultBranch string    `json:"default_branch" db:"default_branch"`
	Stack         string    `json:"stack" db:"stack"`
	AutonomyLevel string    `json:"autonomy_level" db:"autonomy_level"`
	TestCommand   string    `json:"test_command" db:"test_command"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

type Task struct {
	ID                   int64     `json:"id" db:"id"`
	ProjectID            int64     `json:"project_id" db:"project_id"`
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

type Worker struct {
	ID              int64     `json:"id" db:"id"`
	Name            string    `json:"name" db:"name"`
	Capabilities    []string  `json:"capabilities" db:"capabilities"`
	Status          string    `json:"status" db:"status"`
	LastHeartbeatAt time.Time `json:"last_heartbeat_at" db:"last_heartbeat_at"`
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
}

type ApprovalRequest struct {
	ID             int64     `json:"id" db:"id"`
	ProjectID      int64     `json:"project_id" db:"project_id"`
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

type CreateTask struct {
	Project              string   `json:"project"` // slug
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
	Task    Task    `json:"task"`
	Project Project `json:"project"`
}

type Transition struct {
	To    string `json:"to"`
	Error string `json:"error,omitempty"`
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
}

type FinishRun struct {
	ExitStatus int     `json:"exit_status"`
	LogPath    string  `json:"log_path"`
	Tokens     int64   `json:"tokens"`
	CostUSD    float64 `json:"cost_usd"`
}

type Decision struct {
	Comment string `json:"comment"`
}

type ID struct {
	ID int64 `json:"id"`
}
