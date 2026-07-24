package server

import (
	"context"
	"encoding/json"

	toolapp "github.com/Tangerg/lynx/app/runtime/internal/application/tools"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ListTools surfaces every read-only diagnostic tool valid outside an agent
// turn (API.md §7.6).
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

// InvokeTool runs one diagnostic tool directly outside a run. The application
// admits cwd before the adapter confines tool paths beneath it; the result is
// a canonical JSON value projected only at this Delivery boundary.
func (s *Server) InvokeTool(ctx context.Context, in protocol.InvokeToolRequest) (any, error) {
	args, err := json.Marshal(in.Arguments)
	if err != nil {
		return nil, err
	}
	result, err := s.tools.Invoke(ctx, toolapp.Invocation{Name: in.Name, Arguments: string(args), Cwd: in.Cwd})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	return result.Any(), nil
}
