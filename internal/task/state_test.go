package task

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{Pending, Assigned, true},
		{Assigned, Running, true},
		{Running, Testing, true},
		{Testing, WaitingForHuman, true},
		{WaitingForHuman, Pending, true},
		{Running, Pending, true}, // crash requeue
		{Failed, Pending, true},  // retry
		{Pending, Running, false},
		{Pending, Completed, false},
		{WaitingForHuman, Completed, false}, // must resume through a worker
		{Completed, Pending, false},
		{Cancelled, Pending, false},
		{"BOGUS", Pending, false},
		{Pending, "BOGUS", false},
	}
	for _, tt := range tests {
		if got := CanTransition(tt.from, tt.to); got != tt.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestEveryStateHasRules(t *testing.T) {
	for _, s := range []string{Pending, Assigned, Running, Testing, Reviewing, WaitingForHuman, Completed, Failed, Cancelled} {
		if _, ok := transitions[s]; !ok {
			t.Errorf("no transition rules for %s", s)
		}
	}
}
