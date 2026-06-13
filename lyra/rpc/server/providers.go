package server

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/config"
	"github.com/Tangerg/lynx/lyra/internal/domain/provider"
	"github.com/Tangerg/lynx/lyra/internal/domain/tool"
	"github.com/Tangerg/lynx/lyra/rpc/protocol"
	"github.com/Tangerg/lynx/models/catalog"
)

// ListProviders reports the full supported-provider set (the providers Lyra
// has an adapter for), each annotated from the registry: enabled ⇔ a masked
// key is present (API.md §4.9 / §7.6). The per-provider model list isn't
// here — it unlocks via models.list.
func (s *Server) ListProviders(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Provider], error) {
	configured, err := s.rt.Providers().List(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]provider.Provider, len(configured))
	for _, p := range configured {
		byID[p.ID] = p
	}
	supported := config.SupportedProviders()
	out := make([]protocol.Provider, 0, len(supported))
	for _, sp := range supported {
		id := string(sp)
		out = append(out, providerToWire(id, byID[id])) // zero entry when unconfigured
	}
	return protocol.NewPage(out), nil
}

// providerToWire maps a registry entry onto the wire Provider shape: id
// doubles as type; the key is masked ("" = unconfigured, the enabled signal).
func providerToWire(id string, entry provider.Provider) protocol.Provider {
	return protocol.Provider{
		ID:           id,
		Type:         id,
		BaseURL:      entry.BaseURL,
		APIKeyMasked: entry.MaskedAPIKey(),
	}
}

// ConfigureProvider upserts a provider's credentials (key + base URL) into
// the registry and returns the masked result (API.md §7.6). The provider
// must be one Lyra supports.
func (s *Server) ConfigureProvider(ctx context.Context, in protocol.ConfigureProviderRequest) (*protocol.Provider, error) {
	if !isSupportedProvider(in.Provider) {
		return nil, protocol.ErrInvalidParams
	}
	if err := s.rt.Providers().Configure(ctx, provider.Provider{
		ID:      in.Provider,
		APIKey:  in.APIKey,
		BaseURL: in.BaseURL,
	}); err != nil {
		return nil, err
	}
	entry, _, err := s.rt.Providers().Get(ctx, in.Provider)
	if err != nil {
		return nil, err
	}
	out := providerToWire(entry.ID, entry)
	return &out, nil
}

// TestProvider probes a configured provider with a minimal (max_tokens=1)
// completion to validate its key + endpoint (API.md §7.6). Returns
// {ok:false, error} on failure rather than erroring the RPC, so the UI can
// show "test failed: <reason>" inline.
func (s *Server) TestProvider(ctx context.Context, providerID string) (*protocol.ProviderTestResult, error) {
	entry, ok, err := s.rt.Providers().Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !ok || !entry.Enabled() {
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: "provider_not_configured", Detail: "set the API key first",
		}}, nil
	}
	// The build-client + ping lives on the runtime, which owns client
	// construction (clientResolver); this layer just maps the verdict to wire.
	if err := s.rt.ProbeProvider(ctx, entry); err != nil {
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: "provider_test_failed", Detail: err.Error(),
		}}, nil
	}
	return &protocol.ProviderTestResult{OK: true}, nil
}

// ListModels enumerates the models a provider offers, from the embedded
// catalog with full metadata (context window, capabilities, pricing). Served
// straight from the static catalog — no key required (API.md §7.6).
func (s *Server) ListModels(_ context.Context, in protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	models := catalog.Models(in.Provider)
	out := make([]protocol.Model, 0, len(models))
	for _, m := range models {
		out = append(out, modelToWire(in.Provider, m))
	}
	return protocol.NewPage(out), nil
}

// modelToWire maps a catalog model onto the wire Model shape (API.md §4.9).
func modelToWire(providerID string, m chat.ModelInfo) protocol.Model {
	out := protocol.Model{
		ID:              m.ID,
		Provider:        providerID,
		DisplayName:     m.DisplayName,
		ContextWindow:   int(m.Limits.ContextWindow),
		MaxOutputTokens: int(m.Limits.MaxOutputTokens),
		Capabilities: &protocol.ModelCapabilities{
			Reasoning:  m.Reasoning.Supported,
			Multimodal: len(m.Modalities.Input) > 1,
			ToolUse:    m.ToolCall,
		},
	}
	if len(m.Pricing) > 0 {
		out.Pricing = &protocol.ModelPricing{
			InputUsdPerMillionTokens:  m.Pricing[0].InputPer1M,
			OutputUsdPerMillionTokens: m.Pricing[0].OutputPer1M,
		}
	}
	return out
}

// isSupportedProvider reports whether id names a provider Lyra has an adapter
// for — the guard providers.configure uses to reject unknown providers.
func isSupportedProvider(id string) bool {
	return slices.Contains(config.SupportedProviders(), config.Provider(id))
}

// ListTools surfaces every tool the engine registered — built-in coding
// tools plus any MCP-server tools dialed at boot (API.md §7.6).
func (s *Server) ListTools(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.ToolSpec], error) {
	internal, err := s.rt.Tool().List(ctx)
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
	return protocol.NewPage(out), nil
}

// InvokeTool runs one tool directly, outside a run (diagnostics /
// client-driven workflows, API.md §7.6). Backed by tool.Service.Invoke,
// whose result is the tool's raw string output.
func (s *Server) InvokeTool(ctx context.Context, in protocol.InvokeToolRequest) (any, error) {
	args, err := json.Marshal(in.Arguments)
	if err != nil {
		return nil, err
	}
	return s.rt.Tool().Invoke(ctx, in.Name, string(args))
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
