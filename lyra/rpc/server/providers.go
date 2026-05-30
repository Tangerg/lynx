package server

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/lyra/internal/service/tool"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
)

// ListProviders — provider registry isn't part of the engine yet
// (chat.Client is a single configured provider). Return an empty
// list rather than not-implemented so frontends can render an empty
// state instead of an error banner.
func (i *Server) ListProviders(_ context.Context) ([]protocol.Provider, error) {
	return []protocol.Provider{}, nil
}

func (i *Server) TestProvider(_ context.Context, _ string) (*protocol.ProviderTestResult, error) {
	return nil, notImpl("providers.test")
}

// ConfigureProvider — provider registry isn't part of the engine yet
// (chat.Client is a single configured provider via env). Not implemented
// until the registry lands; dispatch maps this to -32601 so the client
// sees an honest "not supported on this build".
func (i *Server) ConfigureProvider(_ context.Context, _ protocol.ConfigureProviderRequest) (*protocol.Provider, error) {
	return nil, notImpl("providers.configure")
}

func (i *Server) ListModels(_ context.Context, _ string) ([]protocol.Model, error) {
	return []protocol.Model{}, nil
}

// ListTools surfaces every tool the engine registered — built-in
// coding tools plus any MCP-server tools dialed at boot.
func (i *Server) ListTools(ctx context.Context) ([]protocol.Tool, error) {
	internal, err := i.rt.Tool().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Tool, 0, len(internal))
	for _, t := range internal {
		schema := json.RawMessage(t.Schema)
		if len(schema) == 0 {
			schema = json.RawMessage(`{}`)
		}
		out = append(out, protocol.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
			SafetyClass: safetyClassToString(t.SafetyClass),
			Origin:      "server",
		})
	}
	return out, nil
}

// InvokeTool runs one tool directly, outside a chat turn (diagnostics /
// client-driven workflows). Backed by tool.Service.Invoke.
func (i *Server) InvokeTool(ctx context.Context, in protocol.InvokeToolRequest) (*protocol.InvokeToolResponse, error) {
	out, err := i.rt.Tool().Invoke(ctx, in.Name, in.Arguments)
	if err != nil {
		return nil, err
	}
	return &protocol.InvokeToolResponse{Output: out}, nil
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
