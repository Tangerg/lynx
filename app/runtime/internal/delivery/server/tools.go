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
			Parameters:  t.Schema.Map(),
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
