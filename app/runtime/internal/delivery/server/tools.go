package server

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ListTools surfaces every tool the engine registered — built-in coding
// tools plus any MCP-server tools dialed at boot (API.md §7.6).
func (s *Server) ListTools(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.ToolSpec], error) {
	internal, err := s.tools.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ToolSpec, 0, len(internal))
	for _, t := range internal {
		out = append(out, protocol.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  parseSchema(t.Schema),
			SafetyClass: presentSafetyClass(t.SafetyClass),
		})
	}
	return protocol.NewPage(out), nil
}

// InvokeTool runs one tool directly, outside a run (diagnostics /
// client-driven workflows, API.md §7.6). Backed by tool.Invoker.Invoke,
// whose result is the tool's raw string output.
func (s *Server) InvokeTool(ctx context.Context, in protocol.InvokeToolRequest) (any, error) {
	args, err := json.Marshal(in.Arguments)
	if err != nil {
		return nil, err
	}
	return s.tools.Invoke(ctx, in.Name, string(args))
}

// parseSchema decodes a tool's JSON Schema string into a structured
// object; an empty / unparseable schema becomes an empty object.
func parseSchema(raw string) map[string]any {
	if raw == "" {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal([]byte(raw), &m) != nil {
		return map[string]any{}
	}
	return m
}
