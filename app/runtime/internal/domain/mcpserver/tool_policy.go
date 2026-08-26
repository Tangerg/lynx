package mcpserver

// ToolPolicy is the effective per-tool policy derived from enabled MCP server
// registrations. It is immutable after construction and safe for concurrent
// readers.
type ToolPolicy struct {
	enabled      map[string]struct{}
	disabled     map[ToolRef]struct{}
	autoApproved map[ToolRef]struct{}
}

// NewToolPolicy derives the effective tool policy from enabled servers.
func NewToolPolicy(servers []Server) ToolPolicy {
	var policy ToolPolicy
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		if policy.enabled == nil {
			policy.enabled = map[string]struct{}{}
		}
		policy.enabled[server.Name] = struct{}{}
		for _, tool := range server.DisabledTools {
			if policy.disabled == nil {
				policy.disabled = map[ToolRef]struct{}{}
			}
			policy.disabled[ToolRef{Server: server.Name, Tool: tool}] = struct{}{}
		}
		for _, tool := range server.AutoApproveTools {
			if policy.autoApproved == nil {
				policy.autoApproved = map[ToolRef]struct{}{}
			}
			policy.autoApproved[ToolRef{Server: server.Name, Tool: tool}] = struct{}{}
		}
	}
	return policy
}

// Disabled reports whether ref is hidden from resolution.
func (t ToolPolicy) Disabled(ref ToolRef) bool {
	if _, ok := t.enabled[ref.Server]; !ok {
		return true
	}
	_, ok := t.disabled[ref]
	return ok
}

// AutoApproved reports whether ref may skip the interactive
// approval prompt after standing approval rules have been evaluated.
func (t ToolPolicy) AutoApproved(ref ToolRef) bool {
	_, ok := t.autoApproved[ref]
	return ok
}
