package protocol

import "context"

// Approval is the approval.* method group (API.md §C.3) — the runtime's
// tool-permission stance. It is a runtime-global policy (not per-session),
// orthogonal to [Item]'s per-tool safetyClass; the two together decide
// whether one call parks for approval.
type Approval interface {
	// GetApprovalMode returns the current runtime tool-permission stance.
	GetApprovalMode(ctx context.Context) (*ApprovalModeResult, error)
	// SetApprovalMode sets the runtime tool-permission stance. plan is the
	// read-only planning stance (the agent investigates + proposes a plan;
	// exit_plan_mode flips back to execute).
	SetApprovalMode(ctx context.Context, in SetApprovalModeRequest) (*ApprovalModeResult, error)

	// ListApprovalRules lists the persisted "remember this decision" rules
	// visible from a session — its session rules, its project's rules, and all
	// global rules (AUX_API §6). The session id resolves the project directory.
	ListApprovalRules(ctx context.Context, in ListApprovalRulesRequest) (*ListApprovalRulesResult, error)

	// ForgetApprovalRule removes one persisted approval rule by id. Removing a
	// missing id is not an error.
	ForgetApprovalRule(ctx context.Context, in ForgetApprovalRuleRequest) error
}

// ApprovalRule is one persisted fine-grained approval rule (AUX_API §6). The
// rule auto-resolves a gated tool call when the call's scope matches, the tool
// matches, and the call's per-tool subject (a shell command, an edited file's
// path) matches the Subject glob — so a rule reads "allow `npm run *` in this
// project", not the blunt whole-tool grant.
type ApprovalRule struct {
	ID       string `json:"id"`
	Scope    string `json:"scope"`             // "session" | "project" | "global"
	Tool     string `json:"tool"`              // tool name, e.g. "shell"
	Subject  string `json:"subject,omitempty"` // command / path glob; "" = any arguments
	Dir      string `json:"dir,omitempty"`     // project-scope directory (display only; omitted otherwise)
	Decision string `json:"decision"`          // "allow" | "deny"
}

// ListApprovalRulesRequest — approval.listRules body. SessionID anchors which
// session + project rules are visible (global rules always are).
type ListApprovalRulesRequest struct {
	SessionID string `json:"sessionId"`
}

// ListApprovalRulesResult — the approval.listRules reply.
type ListApprovalRulesResult struct {
	Rules []ApprovalRule `json:"rules"`
}

// ForgetApprovalRuleRequest — approval.forgetRule body.
type ForgetApprovalRuleRequest struct {
	ID string `json:"id"`
}

// ApprovalMode is the runtime tool-permission stance
// (approval.getMode / approval.setMode). It mirrors the engine's approval gate:
//
//	plan      read-only: write/exec/network tools are DENIED (no prompt) so the
//	          agent only investigates + drafts a plan; exit_plan_mode presents
//	          the plan for approval and flips the stance back to execute
//	safe      every write/exec/network tool prompts for approval
//	balanced  write/network auto-allowed; only exec (shell) prompts (the default)
//	yolo      everything auto-allowed
type ApprovalMode string

const (
	ApprovalModeSafe     ApprovalMode = "safe"
	ApprovalModeBalanced ApprovalMode = "balanced"
	ApprovalModeYolo     ApprovalMode = "yolo"
	ApprovalModePlan     ApprovalMode = "plan"
)

// SetApprovalModeRequest — approval.setMode body.
type SetApprovalModeRequest struct {
	Mode ApprovalMode `json:"mode"`
}

// ApprovalModeResult — the approval.getMode / setMode reply: the (new)
// current stance.
type ApprovalModeResult struct {
	Mode ApprovalMode `json:"mode"`
}
