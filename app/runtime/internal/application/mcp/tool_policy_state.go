package mcp

import (
	"sync/atomic"

	"github.com/Tangerg/scope/app/runtime/internal/domain/mcpserver"
)

// ToolPolicyState owns the effective MCP tool-policy snapshot. The application
// coordinator publishes registry-derived replacements; execution consumers read
// only the policy decisions they need.
type ToolPolicyState struct {
	policy atomic.Pointer[mcpserver.ToolPolicy]
}

// NewToolPolicyState builds a live policy with initial as its current snapshot.
func NewToolPolicyState(initial mcpserver.ToolPolicy) *ToolPolicyState {
	state := &ToolPolicyState{}
	state.Replace(initial)
	return state
}

// Replace atomically publishes a registry-derived policy snapshot.
func (t *ToolPolicyState) Replace(policy mcpserver.ToolPolicy) {
	t.policy.Store(&policy)
}

// ToolDisabled reports whether ref is hidden from run.
func (t *ToolPolicyState) ToolDisabled(ref mcpserver.ToolRef) bool {
	if t == nil {
		return false
	}
	policy := t.policy.Load()
	return policy != nil && policy.Disabled(ref)
}

// ToolAutoApproved reports whether ref may skip an interactive prompt.
func (t *ToolPolicyState) ToolAutoApproved(ref mcpserver.ToolRef) bool {
	if t == nil {
		return false
	}
	policy := t.policy.Load()
	return policy != nil && policy.AutoApproved(ref)
}
