package server

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/lyra/internal/service/tool"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListProviders — provider registry isn't part of the engine yet
// (chat.Client is a single configured provider via env / config.yaml).
// Empty list so the frontend renders an empty state, not an error.
func (i *Server) ListProviders(_ context.Context) ([]protocol.Provider, error) {
	return []protocol.Provider{}, nil
}

// ConfigureProvider — no provider registry to write into yet.
func (i *Server) ConfigureProvider(_ context.Context, _ protocol.ConfigureProviderRequest) (*protocol.Provider, error) {
	return nil, notImpl("providers.configure")
}

// TestProvider — no provider registry to probe yet.
func (i *Server) TestProvider(_ context.Context, _ string) (*protocol.ProviderTestResult, error) {
	return nil, notImpl("providers.test")
}

// ListModels — model catalog isn't enumerated from the engine yet.
func (i *Server) ListModels(_ context.Context, _ string) ([]protocol.Model, error) {
	return []protocol.Model{}, nil
}

// ListTools surfaces every tool the engine registered — built-in coding
// tools plus any MCP-server tools dialed at boot (API.md §7.6).
func (i *Server) ListTools(ctx context.Context) ([]protocol.ToolSpec, error) {
	internal, err := i.rt.Tool().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ToolSpec, 0, len(internal))
	for _, t := range internal {
		out = append(out, protocol.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  parseSchema(t.Schema),
			SafetyClass: safetyClassToString(t.SafetyClass),
		})
	}
	return out, nil
}

// InvokeTool runs one tool directly, outside a run (diagnostics /
// client-driven workflows, API.md §7.6). Backed by tool.Service.Invoke,
// whose result is the tool's raw string output.
func (i *Server) InvokeTool(ctx context.Context, in protocol.InvokeToolRequest) (any, error) {
	args, err := json.Marshal(in.Arguments)
	if err != nil {
		return nil, err
	}
	return i.rt.Tool().Invoke(ctx, in.Name, string(args))
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

func safetyClassToString(c tool.SafetyClass) string {
	switch c {
	case tool.SafetyClassSafe:
		return "safe"
	case tool.SafetyClassWrite:
		return "write"
	case tool.SafetyClassExec:
		return "exec"
	case tool.SafetyClassNetwork:
		return "network"
	default:
		return "safe"
	}
}
