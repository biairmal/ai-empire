// Package policy decides which actions need human approval (spec §9–§11).
package policy

import "strings"

// Actions from spec §10.
const (
	// Low risk
	ReadSource     = "read_source"
	CreateWorktree = "create_worktree"
	ModifyCode     = "modify_code"
	RunTests       = "run_tests"
	Commit         = "commit"
	PushBranch     = "push_branch"

	// Medium risk
	SchemaChange       = "schema_change"
	APIChange          = "api_change"
	AddDependency      = "add_dependency"
	AuthChange         = "auth_change"
	InfraConfig        = "infra_config"
	ArchitectureChange = "architecture_change"

	// High risk
	MergeProtected       = "merge_protected"
	DeployProduction     = "deploy_production"
	DestructiveMigration = "destructive_migration"
	DestructiveInfra     = "destructive_infra"
	SecurityPolicyChange = "security_policy_change"
	GlobalKnowledgePromo = "global_knowledge_promotion"
	PermissionEscalation = "permission_escalation"
)

var low = set(ReadSource, CreateWorktree, ModifyCode, RunTests, Commit, PushBranch,
	"format_code", "static_analysis", "generate_docs", "refactor")

var medium = set(SchemaChange, APIChange, AddDependency, AuthChange, InfraConfig, ArchitectureChange)

var high = set(MergeProtected, DeployProduction, DestructiveMigration, DestructiveInfra,
	SecurityPolicyChange, GlobalKnowledgePromo, PermissionEscalation)

// Medium-risk actions each autonomy preset lets through without approval (spec §11).
// ponytail: presets only; add per-project overrides when a project needs one.
var mediumAuto = map[string]map[string]bool{
	"high":         medium,
	"medium":       set(AddDependency, InfraConfig),
	"conservative": {},
}

// Requires reports whether action needs human approval under an autonomy level.
// Unknown actions and unknown levels fail closed.
func Requires(autonomy, action string) bool {
	switch {
	case low[action]:
		return false
	case medium[action]:
		return !mediumAuto[autonomy][action]
	default: // high risk or unknown
		return true
	}
}

// Known reports whether action is a recognised policy action.
func Known(action string) bool { return low[action] || medium[action] || high[action] }

// CanDecide enforces "AI cannot approve its own work" (spec §9, §26):
// only a human may decide, and an AI actor never decides its own request.
// A human owner may approve what they submitted themselves: they are the final authority.
func CanDecide(requestedBy, decidedBy string) bool {
	return IsHuman(decidedBy) && (decidedBy != requestedBy || IsHuman(requestedBy))
}

// IsHuman reports whether an actor string ("owner", "owner via hermes", "worker:3") is a human.
func IsHuman(actor string) bool {
	return actor == "owner" || strings.HasPrefix(actor, "owner:") || strings.HasPrefix(actor, "owner via ")
}

func set(xs ...string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
