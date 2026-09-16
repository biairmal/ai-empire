package policy

import "testing"

func TestRequires(t *testing.T) {
	tests := []struct {
		autonomy, action string
		want             bool
	}{
		{"conservative", ModifyCode, false},
		{"conservative", AddDependency, true},
		{"medium", AddDependency, false},
		{"medium", SchemaChange, true},
		{"high", SchemaChange, false},
		// High risk always needs a human, whatever the preset.
		{"high", MergeProtected, true},
		{"high", DeployProduction, true},
		{"high", PermissionEscalation, true},
		// Fail closed.
		{"high", "rm_rf_everything", true},
		{"bogus", SchemaChange, true},
		{"bogus", ModifyCode, false},
	}
	for _, tt := range tests {
		if got := Requires(tt.autonomy, tt.action); got != tt.want {
			t.Errorf("Requires(%s, %s) = %v, want %v", tt.autonomy, tt.action, got, tt.want)
		}
	}
}

func TestCanDecide(t *testing.T) {
	tests := []struct {
		requestedBy, decidedBy string
		want                   bool
	}{
		{"worker:1", "owner", true},
		{"worker:1", "worker:1", false}, // self-approval
		{"worker:1", "worker:2", false}, // AI approving AI
		{"owner", "owner", false},       // requester can't decide
		{"worker:1", "hermes", false},
	}
	for _, tt := range tests {
		if got := CanDecide(tt.requestedBy, tt.decidedBy); got != tt.want {
			t.Errorf("CanDecide(%s, %s) = %v, want %v", tt.requestedBy, tt.decidedBy, got, tt.want)
		}
	}
}
