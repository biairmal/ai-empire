// Package task holds the task lifecycle rules (spec §30).
package task

import "slices"

const (
	Pending         = "PENDING"
	Assigned        = "ASSIGNED"
	Running         = "RUNNING"
	Testing         = "TESTING"
	Reviewing       = "REVIEWING"
	WaitingForHuman = "WAITING_FOR_HUMAN"
	Completed       = "COMPLETED"
	Failed          = "FAILED"
	Cancelled       = "CANCELLED"
)

// Stages: where a worker resumes after a gate.
const (
	StageImplement = "implement"
	StageMerge     = "merge"
)

// MaxAttempts is how many times a task may be claimed before requeueing fails it.
const MaxAttempts = 3

// Pending is reachable from in-flight states so crashed work can be requeued.
var transitions = map[string][]string{
	Pending:         {Assigned, Cancelled},
	Assigned:        {Running, Pending, Failed, Cancelled},
	Running:         {Testing, Reviewing, WaitingForHuman, Completed, Pending, Failed, Cancelled},
	Testing:         {Running, Reviewing, WaitingForHuman, Completed, Pending, Failed, Cancelled},
	Reviewing:       {Running, WaitingForHuman, Completed, Pending, Failed, Cancelled},
	WaitingForHuman: {Pending, Cancelled, Completed}, // Completed: a document approval finishes the task
	Failed:          {Pending},
	Completed:       {},
	Cancelled:       {},
}

// CanTransition reports whether a task may move from one status to another.
func CanTransition(from, to string) bool {
	return slices.Contains(transitions[from], to)
}

// InFlight reports whether a worker is currently holding a task in this status.
func InFlight(status string) bool {
	switch status {
	case Assigned, Running, Testing, Reviewing:
		return true
	}
	return false
}
