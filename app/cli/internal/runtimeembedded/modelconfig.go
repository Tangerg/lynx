package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/modelconfig"
)

type modelConfigBinding interface {
	GetUtilityRole(context.Context, embedded.CallOptions) (*protocol.UtilityRole, error)
	SetUtilityRole(context.Context, protocol.UtilityRole, embedded.CommandOptions) (*protocol.UtilityRole, error)
	GetEmbeddingRole(context.Context, embedded.CallOptions) (*protocol.EmbeddingRole, error)
	SetEmbeddingRole(context.Context, protocol.EmbeddingRole, embedded.CommandOptions) (*protocol.EmbeddingRole, error)
	ListProviders(context.Context, embedded.CallOptions) (*protocol.Page[protocol.Provider], error)
	UpdateProvider(context.Context, protocol.UpdateProviderRequest, embedded.CommandOptions) (*protocol.Provider, error)
	TestProvider(context.Context, protocol.TestProviderRequest, embedded.CallOptions) (*protocol.ProviderTestResult, error)
}

var _ modelconfig.Service = (*Runtime)(nil)

func (r *Runtime) Roles(ctx context.Context) (modelconfig.Roles, error) {
	utility, err := r.modelConfig.GetUtilityRole(ctx, r.callOptions())
	if err != nil {
		return modelconfig.Roles{}, classifyError(err)
	}
	if utility == nil {
		return modelconfig.Roles{}, errors.New("model roles: runtime returned nil utility role")
	}
	embedding, err := r.modelConfig.GetEmbeddingRole(ctx, r.callOptions())
	if err != nil {
		return modelconfig.Roles{}, classifyError(err)
	}
	if embedding == nil {
		return modelconfig.Roles{}, errors.New("model roles: runtime returned nil embedding role")
	}
	roles := modelconfig.Roles{
		Utility:   modelconfig.Role{Kind: modelconfig.UtilityRole, Provider: utility.Provider, Model: utility.Model},
		Embedding: modelconfig.Role{Kind: modelconfig.EmbeddingRole, Provider: embedding.Provider, Model: embedding.Model},
	}
	if err := roles.Validate(); err != nil {
		return modelconfig.Roles{}, fmt.Errorf("model roles: %w", err)
	}
	return roles, nil
}

func (r *Runtime) SetRole(ctx context.Context, role modelconfig.Role) (modelconfig.Role, error) {
	if err := role.Validate(); err != nil {
		return modelconfig.Role{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return modelconfig.Role{}, err
	}
	var projected modelconfig.Role
	switch role.Kind {
	case modelconfig.UtilityRole:
		result, callErr := r.modelConfig.SetUtilityRole(ctx, protocol.UtilityRole{Provider: role.Provider, Model: role.Model}, options)
		if callErr != nil {
			return modelconfig.Role{}, classifyError(callErr)
		}
		if result == nil {
			return modelconfig.Role{}, errors.New("set utility role: runtime returned nil")
		}
		projected = modelconfig.Role{Kind: role.Kind, Provider: result.Provider, Model: result.Model}
	case modelconfig.EmbeddingRole:
		result, callErr := r.modelConfig.SetEmbeddingRole(ctx, protocol.EmbeddingRole{Provider: role.Provider, Model: role.Model}, options)
		if callErr != nil {
			return modelconfig.Role{}, classifyError(callErr)
		}
		if result == nil {
			return modelconfig.Role{}, errors.New("set embedding role: runtime returned nil")
		}
		projected = modelconfig.Role{Kind: role.Kind, Provider: result.Provider, Model: result.Model}
	}
	if err := projected.Validate(); err != nil {
		return modelconfig.Role{}, fmt.Errorf("set model role: %w", err)
	}
	return projected, nil
}

func (r *Runtime) Providers(ctx context.Context) ([]modelconfig.Provider, error) {
	result, err := r.modelConfig.ListProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	if result == nil {
		return nil, errors.New("list providers: runtime returned nil")
	}
	if result.NextCursor != "" {
		return nil, errors.New("list providers: runtime returned an unsupported continuation cursor")
	}
	providers := make([]modelconfig.Provider, 0, len(result.Data))
	seen := make(map[string]struct{}, len(result.Data))
	for index, value := range result.Data {
		provider := projectProvider(value)
		if err := provider.Validate(); err != nil {
			return nil, fmt.Errorf("list providers item %d: %w", index+1, err)
		}
		if _, duplicate := seen[provider.ID]; duplicate {
			return nil, fmt.Errorf("list providers repeats %q", provider.ID)
		}
		seen[provider.ID] = struct{}{}
		providers = append(providers, provider)
	}
	return providers, nil
}

func (r *Runtime) UpdateProvider(ctx context.Context, update modelconfig.UpdateProvider) (modelconfig.Provider, error) {
	if err := update.Validate(); err != nil {
		return modelconfig.Provider{}, err
	}
	options, err := r.commandOptions()
	if err != nil {
		return modelconfig.Provider{}, err
	}
	request := protocol.UpdateProviderRequest{Provider: update.Provider}
	request.BaseURL = projectProviderChange(update.BaseURL)
	request.APIKey = projectProviderChange(update.APIKey)
	result, err := r.modelConfig.UpdateProvider(ctx, request, options)
	if err != nil {
		return modelconfig.Provider{}, classifyError(err)
	}
	if result == nil {
		return modelconfig.Provider{}, errors.New("update provider: runtime returned nil")
	}
	provider := projectProvider(*result)
	if err := provider.Validate(); err != nil {
		return modelconfig.Provider{}, fmt.Errorf("update provider: %w", err)
	}
	return provider, nil
}

func (r *Runtime) TestProvider(ctx context.Context, providerID string) (modelconfig.TestResult, error) {
	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return modelconfig.TestResult{}, errors.New("test provider: provider id is empty")
	}
	result, err := r.modelConfig.TestProvider(ctx, protocol.TestProviderRequest{Provider: providerID}, r.callOptions())
	if err != nil {
		return modelconfig.TestResult{}, classifyError(err)
	}
	if result == nil {
		return modelconfig.TestResult{}, errors.New("test provider: runtime returned nil")
	}
	projected := modelconfig.TestResult{OK: result.OK}
	if result.Error != nil {
		projected.Problem = result.Error.Type
		if result.Error.Detail != "" {
			projected.Problem += ": " + result.Error.Detail
		}
	}
	if err := projected.Validate(); err != nil {
		return modelconfig.TestResult{}, fmt.Errorf("test provider: %w", err)
	}
	return projected, nil
}

func projectProvider(value protocol.Provider) modelconfig.Provider {
	return modelconfig.Provider{
		ID: value.ID, BaseURL: value.BaseURL, APIKeyMasked: value.APIKeyMasked,
		KeySource: modelconfig.KeySource(value.KeySource), RequiresBaseURL: value.RequiresBaseURL,
		EmbeddingCapable: value.EmbeddingCapable, DefaultEmbeddingModel: value.DefaultEmbeddingModel,
	}
}

func projectProviderChange(change *modelconfig.ValueChange) *protocol.ProviderConfigChange {
	if change == nil {
		return nil
	}
	projected := &protocol.ProviderConfigChange{Type: protocol.ProviderConfigChangeType(change.Kind)}
	if change.Kind == modelconfig.SetValue {
		value := change.Value
		projected.Value = &value
	}
	return projected
}
