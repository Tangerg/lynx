package integrations

import (
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/mcpserver"
)

// ToolPolicyState owns the effective MCP tool-policy snapshot. The application
// coordinator publishes registry-derived replacements; execution adapters read
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
func (s *ToolPolicyState) Replace(policy mcpserver.ToolPolicy) {
	s.policy.Store(&policy)
}

// ToolDisabled reports whether ref is hidden from execution.
func (s *ToolPolicyState) ToolDisabled(ref mcpserver.ToolRef) bool {
	if s == nil {
		return false
	}
	policy := s.policy.Load()
	return policy != nil && policy.Disabled(ref)
}

// ToolAutoApproved reports whether ref may skip an interactive prompt.
func (s *ToolPolicyState) ToolAutoApproved(ref mcpserver.ToolRef) bool {
	if s == nil {
		return false
	}
	policy := s.policy.Load()
	return policy != nil && policy.AutoApproved(ref)
}
