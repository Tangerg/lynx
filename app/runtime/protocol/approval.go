package protocol

// ApprovalRule is one persisted fine-grained approval rule (AUX_API §6). The
// rule auto-resolves a gated tool call when the call's scope matches, the tool
// matches, and the call's per-tool subject (a shell command, an edited file's
// path) matches the Subject glob — so a rule reads "allow `npm run *` in this
// project", not the blunt whole-tool grant.
type ApprovalRule struct {
	ID       string               `json:"id"`
	Scope    ApprovalRuleScope    `json:"scope"`
	Tool     string               `json:"tool"`              // tool name, e.g. "shell"
	Subject  string               `json:"subject,omitempty"` // command / path glob; "" = any arguments
	Dir      string               `json:"dir,omitempty"`     // project-scope directory (display only; omitted otherwise)
	Decision ApprovalRuleDecision `json:"decision"`
}

// ApprovalRuleScope is how far a remembered tool decision reaches.
type ApprovalRuleScope string

const (
	ApprovalRuleScopeSession ApprovalRuleScope = "session"
	ApprovalRuleScopeProject ApprovalRuleScope = "project"
	ApprovalRuleScopeGlobal  ApprovalRuleScope = "global"
)

// ApprovalRuleDecision is the only persisted rule verdict. It is distinct from
// an interrupt-response decision: one is a durable policy record, the other is
// a reply to a single pending approval.
type ApprovalRuleDecision string

const (
	ApprovalRuleDecisionAllow ApprovalRuleDecision = "allow"
	ApprovalRuleDecisionDeny  ApprovalRuleDecision = "deny"
)

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

// ApprovalMode is the runtime's default tool-permission stance
// (approval.getMode / approval.setMode). It mirrors the engine's approval gate:
//
//	safe      every write/exec/network tool prompts for approval
//	balanced  write/network auto-allowed; only exec (shell) prompts (the default)
//	yolo      everything auto-allowed
type ApprovalMode string

const (
	ApprovalModeSafe     ApprovalMode = "safe"
	ApprovalModeBalanced ApprovalMode = "balanced"
	ApprovalModeYolo     ApprovalMode = "yolo"
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
