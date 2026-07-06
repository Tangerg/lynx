package server

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
)

// ListProviders reports the full supported-provider set (the providers Lyra
// has an adapter for), each annotated from the registry: enabled ⇔ a masked
// key is present (API.md §4.9 / §7.6). The per-provider model list isn't
// here — it unlocks via models.list.
func (s *Server) ListProviders(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Provider], error) {
	configured, err := s.providers.ListRegisteredProviders(ctx)
	if err != nil {
		return nil, err
	}
	return protocol.NewPage(providerListWire(configured, s.providers.SupportedProviders())), nil
}

// ConfigureProvider upserts a provider's credentials (key + base URL) into
// the registry and returns the masked result (API.md §7.6). The provider
// must be one Lyra supports.
func (s *Server) ConfigureProvider(ctx context.Context, in protocol.ConfigureProviderRequest) (*protocol.Provider, error) {
	meta, ok := s.providers.ProviderMetadata(in.Provider)
	if !ok {
		return nil, protocol.ErrInvalidParams
	}
	if meta.RequiresBaseURL && in.BaseURL == "" {
		return nil, protocol.ErrInvalidParams
	}
	if err := s.providers.ConfigureProvider(ctx, provider.Provider{
		ID:      in.Provider,
		APIKey:  in.APIKey,
		BaseURL: in.BaseURL,
	}); err != nil {
		return nil, err
	}
	entry, ok, err := s.providers.GetRegisteredProvider(ctx, in.Provider)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("configured provider not found")
	}
	out := providerToWire(meta, entry)
	return &out, nil
}

// TestProvider probes a configured provider with a minimal (max_tokens=1)
// completion to validate its key + endpoint (API.md §7.6). Returns
// {ok:false, error} on failure rather than erroring the RPC, so the UI can
// show "test failed: <reason>" inline.
func (s *Server) TestProvider(ctx context.Context, providerID string) (*protocol.ProviderTestResult, error) {
	entry, ok, err := s.providers.GetRegisteredProvider(ctx, providerID)
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
	if err := s.providers.ProbeProvider(ctx, entry); err != nil {
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: "provider_test_failed", Detail: err.Error(),
		}}, nil
	}
	return &protocol.ProviderTestResult{OK: true}, nil
}
