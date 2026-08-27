package runtimeembedded

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/app/runtime/embedded"
	"github.com/Tangerg/scope/app/runtime/protocol"

	"github.com/Tangerg/scope/app/cli/internal/modelconfig"
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
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned nil utility role")
	}
	embedding, err := r.modelConfig.GetEmbeddingRole(ctx, r.callOptions())
	if err != nil {
		return modelconfig.Roles{}, classifyError(err)
	}
	if embedding == nil {
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned nil embedding role")
	}
	roles := modelconfig.Roles{
		Utility:   modelconfig.Role{Kind: modelconfig.UtilityRole, Provider: utility.Provider, Model: utility.Model},
		Embedding: modelconfig.Role{Kind: modelconfig.EmbeddingRole, Provider: embedding.Provider, Model: embedding.Model},
	}
	if err := roles.Validate(); err != nil {
		return modelconfig.Roles{}, runtimeContractViolation("model roles returned an invalid projection: %v", err)
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
			return modelconfig.Role{}, runtimeContractViolation("set utility role returned nil")
		}
		projected = modelconfig.Role{Kind: role.Kind, Provider: result.Provider, Model: result.Model}
	case modelconfig.EmbeddingRole:
		result, callErr := r.modelConfig.SetEmbeddingRole(ctx, protocol.EmbeddingRole{Provider: role.Provider, Model: role.Model}, options)
		if callErr != nil {
			return modelconfig.Role{}, classifyError(callErr)
		}
		if result == nil {
			return modelconfig.Role{}, runtimeContractViolation("set embedding role returned nil")
		}
		projected = modelconfig.Role{Kind: role.Kind, Provider: result.Provider, Model: result.Model}
	}
	if err := projected.Validate(); err != nil {
		return modelconfig.Role{}, runtimeContractViolation("set model role returned an invalid projection: %v", err)
	}
	if projected != role {
		return modelconfig.Role{}, runtimeContractViolation("set %s role returned %q/%q for %q/%q", role.Kind, projected.Provider, projected.Model, role.Provider, role.Model)
	}
	return projected, nil
}

func (r *Runtime) Providers(ctx context.Context) ([]modelconfig.Provider, error) {
	result, err := r.modelConfig.ListProviders(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list providers", result)
	if err != nil {
		return nil, err
	}
	providers := make([]modelconfig.Provider, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		provider := projectProvider(value)
		if err := provider.Validate(); err != nil {
			return nil, runtimeContractViolation("list providers item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[provider.ID]; duplicate {
			return nil, runtimeContractViolation("list providers repeats %q", provider.ID)
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
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned nil")
	}
	provider := projectProvider(*result)
	if err := provider.Validate(); err != nil {
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned an invalid provider: %v", err)
	}
	if provider.ID != update.Provider {
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned id %q for %q", provider.ID, update.Provider)
	}
	if err := validateProviderUpdate(update, provider); err != nil {
		return modelconfig.Provider{}, runtimeContractViolation("update provider returned an invalid acknowledgement: %v", err)
	}
	return provider, nil
}

func validateProviderUpdate(update modelconfig.UpdateProvider, result modelconfig.Provider) error {
	var problems []error
	if change := update.BaseURL; change != nil {
		want := change.Value
		if change.Kind == modelconfig.ClearValue {
			want = ""
		}
		if result.BaseURL != want {
			problems = append(problems, fmt.Errorf("runtime returned base URL %q, want %q", result.BaseURL, want))
		}
	}
	if change := update.APIKey; change != nil {
		switch change.Kind {
		case modelconfig.SetValue:
			if result.APIKeyMasked == "" || result.KeySource != modelconfig.KeyStored {
				problems = append(problems, fmt.Errorf(
					"runtime returned API key mask %q from %q after setting a stored key",
					result.APIKeyMasked,
					result.KeySource,
				))
			}
			if result.APIKeyMasked == change.Value && change.Value != "****" {
				problems = append(problems, errors.New("runtime exposed the raw API key instead of a mask"))
			}
		case modelconfig.ClearValue:
			// Clearing a stored key may reveal a read-only environment fallback,
			// but the effective credential must no longer claim stored ownership.
			if result.KeySource == modelconfig.KeyStored {
				problems = append(problems, errors.New("runtime still reports a stored API key after clearing it"))
			}
		}
	}
	return errors.Join(problems...)
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
		return modelconfig.TestResult{}, runtimeContractViolation("test provider returned nil")
	}
	projected := modelconfig.TestResult{OK: result.OK, Problem: projectRuntimeProblem(result.Error)}
	if err := projected.Validate(); err != nil {
		return modelconfig.TestResult{}, runtimeContractViolation("test provider returned an invalid result: %v", err)
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
