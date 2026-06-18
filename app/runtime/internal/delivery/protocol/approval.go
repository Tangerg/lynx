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
}

// ApprovalMode is the runtime tool-permission stance
// (approval.getMode / approval.setMode). It mirrors the engine's approval gate:
//
//	plan      read-only: write/exec/network tools are DENIED (no prompt) so the
//	          agent only investigates + drafts a plan; exit_plan_mode presents
//	          the plan for approval and flips the stance back to execute
//	safe      every write/exec/network tool prompts for approval
//	balanced  write/network auto-allowed; only exec (bash) prompts (the default)
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
