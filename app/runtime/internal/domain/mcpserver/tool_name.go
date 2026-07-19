package mcpserver

// ToolRef identifies one remote tool without relying on its sanitized,
// length-limited model-facing name. It is the policy identity: two different
// (server, tool) pairs may legitimately collapse to the same public name.
type ToolRef struct {
	Server string
	Tool   string
}

// PublicName returns the model-facing name for r.
func (r ToolRef) PublicName() string { return ToolName(r.Server, r.Tool) }

// ToolName returns the model-facing name for a tool advertised by an MCP
// server. It matches the name the MCP tool adapter publishes into the model
// tool list, so the runtime can validate the live public catalog without
// importing the concrete MCP adapter. Policy uses [ToolRef], not this lossy
// presentation label.
func ToolName(server, tool string) string {
	if server == "" {
		return sanitizeToolName(tool)
	}
	return sanitizeToolName(server + "_" + tool)
}

func sanitizeToolName(name string) string {
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '_',
			c == '-':
			b = append(b, c)
		default:
			b = append(b, '_')
		}
	}
	if len(b) > 64 {
		b = b[:64]
	}
	return string(b)
}
