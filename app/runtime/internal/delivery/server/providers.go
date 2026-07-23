package server

import (
	"context"
	"errors"

	modelapp "github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// ListProviders projects the application-owned supported-provider set onto the
// protocol page. The application combines static support and runtime state.
func (s *Server) ListProviders(ctx context.Context, _ protocol.PageQuery) (*protocol.Page[protocol.Provider], error) {
	providers, err := s.models.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Provider, 0, len(providers))
	for _, provider := range providers {
		out = append(out, providerToWire(provider))
	}
	return protocol.NewPage(out), nil
}

// ConfigureProvider validates and persists one provider through the application
// use case, then projects its redacted result onto the wire.
func (s *Server) ConfigureProvider(ctx context.Context, in protocol.ConfigureProviderRequest) (*protocol.Provider, error) {
	configured, err := s.models.ConfigureProvider(ctx, modelapp.ConfigureProviderCommand{
		ID:      in.Provider,
		APIKey:  in.APIKey,
		BaseURL: in.BaseURL,
	})
	if err != nil {
		return nil, mapModelError(err)
	}
	out := providerToWire(configured)
	return &out, nil
}

// TestProvider returns an inline verdict for a supported, configured provider.
// The application owns eligibility and probing; Delivery selects the protocol
// failure envelope.
func (s *Server) TestProvider(ctx context.Context, providerID string) (*protocol.ProviderTestResult, error) {
	err := s.models.TestProvider(ctx, providerID)
	if err == nil {
		return &protocol.ProviderTestResult{OK: true}, nil
	}
	if errors.Is(err, modelapp.ErrProviderUnsupported) {
		return nil, mapModelError(err)
	}
	if errors.Is(err, modelapp.ErrProviderUnconfigured) {
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: "provider_not_configured", Detail: "set the API key first",
		}}, nil
	}
	return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
		Type: "provider_test_failed", Detail: err.Error(),
	}}, nil
}
