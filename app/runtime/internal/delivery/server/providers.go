package server

import (
	"context"
	"fmt"

	modelapp "github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListProviders projects the application-owned supported-provider set onto the
// protocol page. The application combines static support and runtime state.
func (s *Server) ListProviders(ctx context.Context) (*protocol.Page[protocol.Provider], error) {
	providers, err := s.models.ListProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.Provider, 0, len(providers))
	for _, provider := range providers {
		wire, err := presentProvider(provider)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return protocol.NewPage(out), nil
}

// UpdateProvider validates and persists one provider through the application
// use case, then projects its redacted result onto the wire.
func (s *Server) UpdateProvider(ctx context.Context, in protocol.UpdateProviderRequest) (*protocol.Provider, error) {
	apiKey, err := providerConfigValue(in.APIKey)
	if err != nil {
		return nil, err
	}
	baseURL, err := providerConfigValue(in.BaseURL)
	if err != nil {
		return nil, err
	}
	configured, err := s.models.UpdateProvider(ctx, modelapp.UpdateProviderCommand{
		ID:      in.Provider,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return nil, mapModelError(err)
	}
	out, err := presentProvider(configured)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func providerConfigValue(change *protocol.ProviderConfigChange) (*string, error) {
	if change == nil {
		return nil, nil
	}
	var value string
	switch change.Type {
	case protocol.ProviderConfigSet:
		if change.Value == nil || *change.Value == "" {
			return nil, protocol.ErrInvalidParams
		}
		value = *change.Value
	case protocol.ProviderConfigClear:
		if change.Value != nil {
			return nil, protocol.ErrInvalidParams
		}
	default:
		return nil, protocol.ErrInvalidParams
	}
	return &value, nil
}

// TestProvider returns an inline verdict for a supported, configured provider.
// The application owns eligibility and probing; Delivery selects the protocol
// failure envelope.
func (s *Server) TestProvider(ctx context.Context, providerID string) (*protocol.ProviderTestResult, error) {
	outcome, err := s.models.TestProvider(ctx, providerID)
	if err != nil {
		return nil, mapModelError(err)
	}
	switch outcome {
	case modelapp.ProviderTestSucceeded:
		return &protocol.ProviderTestResult{OK: true}, nil
	case modelapp.ProviderTestNotConfigured:
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: protocol.ProblemProviderNotConfigured,
		}}, nil
	case modelapp.ProviderTestFailed:
		return &protocol.ProviderTestResult{OK: false, Error: &protocol.ProblemData{
			Type: protocol.ProblemProviderTestFailed,
		}}, nil
	default:
		return nil, fmt.Errorf("server: unknown provider test outcome %q", outcome)
	}
}
