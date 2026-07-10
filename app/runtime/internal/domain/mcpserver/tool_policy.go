package mcpserver

// ToolPolicy is the effective per-tool policy derived from enabled MCP server
// registrations. It is immutable after construction and safe for concurrent
// readers.
type ToolPolicy struct {
	disabled     map[string]struct{}
	autoApproved map[string]struct{}
}

// NewToolPolicy derives the effective tool policy from enabled servers.
func NewToolPolicy(servers []Server) ToolPolicy {
	var policy ToolPolicy
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		for _, tool := range server.DisabledTools {
			if policy.disabled == nil {
				policy.disabled = map[string]struct{}{}
			}
			policy.disabled[ToolName(server.Name, tool)] = struct{}{}
		}
		for _, tool := range server.AutoApproveTools {
			if policy.autoApproved == nil {
				policy.autoApproved = map[string]struct{}{}
			}
			policy.autoApproved[ToolName(server.Name, tool)] = struct{}{}
		}
	}
	return policy
}

// Disabled reports whether the model-facing tool is hidden from resolution.
func (p ToolPolicy) Disabled(toolName string) bool {
	_, ok := p.disabled[toolName]
	return ok
}

// AutoApproved reports whether the model-facing tool may skip the interactive
// approval prompt after standing approval rules have been evaluated.
func (p ToolPolicy) AutoApproved(toolName string) bool {
	_, ok := p.autoApproved[toolName]
	return ok
}
