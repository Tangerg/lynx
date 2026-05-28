package coreimpl

import (
	"context"
	"encoding/json"

	"github.com/Tangerg/lynx/lyra/internal/service/tool"
	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
)

// ListProviders — provider registry isn't part of the engine yet
// (chat.Client is a single configured provider). Return an empty
// list rather than not-implemented so frontends can render an empty
// state instead of an error banner.
func (i *Impl) ListProviders(_ context.Context) ([]coreapi.Provider, error) {
	return []coreapi.Provider{}, nil
}

func (i *Impl) TestProvider(_ context.Context, _ string) (*coreapi.ProviderTestResult, error) {
	return nil, notImpl("providers.test")
}

func (i *Impl) ListModels(_ context.Context, _ string) ([]coreapi.Model, error) {
	return []coreapi.Model{}, nil
}

// ListTools surfaces every tool the engine registered — built-in
// coding tools plus any MCP-server tools dialled at boot.
func (i *Impl) ListTools(ctx context.Context) ([]coreapi.Tool, error) {
	internal, err := i.rt.Tool().List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]coreapi.Tool, 0, len(internal))
	for _, t := range internal {
		schema := json.RawMessage(t.Schema)
		if len(schema) == 0 {
			schema = json.RawMessage(`{}`)
		}
		out = append(out, coreapi.Tool{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
			SafetyClass: safetyClassToString(t.SafetyClass),
			Origin:      "server",
		})
	}
	return out, nil
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
