package server

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/lyra/internal/service/tool"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListProviders surfaces the single configured LLM provider (Lyra talks
// to one provider via config.yaml / env — there is no registry to add to
// yet, hence providers.configure stays notImpl). apiKeyMasked is
// display-safe (API.md §4.9 / §7.6).
func (i *Server) ListProviders(_ context.Context) ([]protocol.Provider, error) {
	provider, _, masked := i.rt.ProviderInfo()
	if provider == "" {
		return []protocol.Provider{}, nil
	}
	return []protocol.Provider{{
		ID:           provider,
		Type:         provider,
		APIKeyMasked: masked,
	}}, nil
}

// ConfigureProvider — no provider registry to write into yet.
func (i *Server) ConfigureProvider(_ context.Context, _ protocol.ConfigureProviderRequest) (*protocol.Provider, error) {
	return nil, notImpl("providers.configure")
}

// TestProvider — no provider registry to probe yet.
func (i *Server) TestProvider(_ context.Context, _ string) (*protocol.ProviderTestResult, error) {
	return nil, notImpl("providers.test")
}

// ListModels surfaces the configured default model (no provider model
// catalog is enumerated yet; the provider filter is ignored). API.md §7.6.
func (i *Server) ListModels(_ context.Context, _ string) ([]protocol.Model, error) {
	provider, model, _ := i.rt.ProviderInfo()
	if model == "" {
		return []protocol.Model{}, nil
	}
	return []protocol.Model{{ID: model, Provider: provider}}, nil
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
