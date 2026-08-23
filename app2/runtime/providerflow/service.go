// Package providerflow owns provider credentials, live probes and model-role
// selection. Vendor wire details remain in llmadapter.
package providerflow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/app2/runtime/domain/provider"
	"github.com/Tangerg/lynx/app2/runtime/llmadapter"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/embedding"
	catalog "github.com/Tangerg/lynx/models/catalog"
)

type Store interface {
	GetProvider(context.Context, string) (provider.Provider, bool, error)
	PutProvider(context.Context, provider.Provider) error
	GetModelRole(context.Context, string, any) (bool, error)
	PutModelRole(context.Context, string, any) error
}

type Service struct {
	store Store
	environment map[string]string
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, errors.New("providerflow: store is required")
	}
	return &Service{store: store, environment: llmadapter.EnvKeys()}, nil
}

func (service *Service) List(ctx context.Context) (*protocol.Page[protocol.Provider], error) {
	providers := llmadapter.SupportedProviders()
	data := make([]protocol.Provider, 0, len(providers))
	for _, providerID := range providers {
		entry, _, err := service.effective(ctx, string(providerID))
		if err != nil {
			return nil, err
		}
		data = append(data, presentProvider(providerID, entry))
	}
	return protocol.NewPage(data), nil
}

func (service *Service) Update(ctx context.Context, request protocol.UpdateProviderRequest) (*protocol.Provider, error) {
	providerID := llmadapter.Provider(request.Provider)
	if !providerID.IsSupported() {
		return nil, fmt.Errorf("%w: unsupported provider %q", protocol.ErrInvalidParams, request.Provider)
	}
	if request.APIKey == nil && request.BaseURL == nil {
		return nil, fmt.Errorf("%w: provider update has no changes", protocol.ErrInvalidParams)
	}
	stored, found, err := service.store.GetProvider(ctx, request.Provider)
	if err != nil {
		return nil, err
	}
	if !found {
		stored.ID = request.Provider
	}
	patch, err := providerPatch(request)
	if err != nil {
		return nil, err
	}
	stored = stored.Apply(patch)
	if err := stored.Validate(providerID.RequiresBaseURL()); err != nil {
		return nil, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	if err := service.store.PutProvider(ctx, stored); err != nil {
		return nil, err
	}
	effective, _, err := service.effective(ctx, request.Provider)
	if err != nil {
		return nil, err
	}
	presented := presentProvider(providerID, effective)
	return &presented, nil
}

func (service *Service) Test(ctx context.Context, id string) (*protocol.ProviderTestResult, error) {
	providerID := llmadapter.Provider(id)
	if !providerID.IsSupported() {
		return nil, fmt.Errorf("%w: unsupported provider %q", protocol.ErrInvalidParams, id)
	}
	entry, configured, err := service.effective(ctx, id)
	if err != nil {
		return nil, err
	}
	if !configured {
		return &protocol.ProviderTestResult{Error: &protocol.ProblemData{Type: protocol.ProblemProviderNotConfigured}}, nil
	}
	if err := probe(ctx, providerID, entry); err != nil {
		return &protocol.ProviderTestResult{Error: &protocol.ProblemData{Type: protocol.ProblemProviderTestFailed}}, nil
	}
	return &protocol.ProviderTestResult{OK: true}, nil
}

func (service *Service) Models(ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	if request.Provider == "" {
		return protocol.NewPage([]protocol.Model{}), nil
	}
	providerID := llmadapter.Provider(request.Provider)
	if !providerID.IsSupported() {
		return protocol.NewPage([]protocol.Model{}), nil
	}
	if providerID.ProbeModels() {
		entry, _, err := service.effective(ctx, request.Provider)
		if err != nil {
			return nil, err
		}
		baseURL := entry.BaseURL
		if baseURL == "" {
			baseURL = providerID.DefaultBaseURL()
		}
		if baseURL != "" {
			ids, err := llmadapter.ListRemoteModels(ctx, baseURL, entry.APIKey)
			if err == nil && len(ids) > 0 {
				data := make([]protocol.Model, 0, len(ids))
				for _, id := range ids {
					if model, found := catalog.Lookup(request.Provider, id); found {
						data = append(data, presentModel(request.Provider, model))
					} else {
						data = append(data, protocol.Model{ID: id, Provider: request.Provider})
					}
				}
				return protocol.NewPage(data), nil
			}
		}
	}
	entries := catalog.Models(request.Provider)
	data := make([]protocol.Model, 0, len(entries))
	for _, entry := range entries {
		data = append(data, presentModel(request.Provider, entry))
	}
	return protocol.NewPage(data), nil
}

func (service *Service) UtilityRole(ctx context.Context) (*protocol.UtilityRole, error) {
	var role protocol.UtilityRole
	if _, err := service.store.GetModelRole(ctx, "utility", &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (service *Service) SetUtilityRole(ctx context.Context, role protocol.UtilityRole) (*protocol.UtilityRole, error) {
	if err := service.validateRole(ctx, role.Provider, role.Model, false); err != nil {
		return nil, err
	}
	if err := service.store.PutModelRole(ctx, "utility", role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (service *Service) EmbeddingRole(ctx context.Context) (*protocol.EmbeddingRole, error) {
	var role protocol.EmbeddingRole
	if _, err := service.store.GetModelRole(ctx, "embedding", &role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (service *Service) SetEmbeddingRole(ctx context.Context, role protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
	if err := service.validateRole(ctx, role.Provider, role.Model, true); err != nil {
		return nil, err
	}
	if err := service.store.PutModelRole(ctx, "embedding", role); err != nil {
		return nil, err
	}
	return &role, nil
}

func (service *Service) validateRole(ctx context.Context, providerID, model string, embedding bool) error {
	if providerID == "" && model == "" {
		return nil
	}
	if providerID == "" || model == "" {
		return fmt.Errorf("%w: provider and model must be set together", protocol.ErrInvalidParams)
	}
	metadata := llmadapter.Provider(providerID)
	if !metadata.IsSupported() || (embedding && !metadata.EmbeddingCapable()) {
		return fmt.Errorf("%w: provider %q cannot serve this role", protocol.ErrInvalidParams, providerID)
	}
	_, configured, err := service.effective(ctx, providerID)
	if err != nil {
		return err
	}
	if !configured {
		return fmt.Errorf("%w: provider %q is not configured", protocol.ErrProviderError, providerID)
	}
	return nil
}

func (service *Service) ResolveClient(ctx context.Context, providerID, model string) (*chatclient.Client, error) {
	entry, configured, err := service.effective(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, fmt.Errorf("%w: provider %q is not configured", protocol.ErrProviderError, providerID)
	}
	return llmadapter.BuildClient(llmadapter.ClientSpec{
		Provider: llmadapter.Provider(providerID), Model: model,
		APIKey: entry.APIKey, BaseURL: entry.BaseURL,
	})
}

func (service *Service) ResolveEmbedding(ctx context.Context) (embedding.Model, protocol.EmbeddingRole, error) {
	role, err := service.EmbeddingRole(ctx)
	if err != nil {
		return nil, protocol.EmbeddingRole{}, err
	}
	if role.Provider == "" || role.Model == "" {
		return nil, *role, fmt.Errorf("%w: configure an embedding model role first", protocol.ErrProviderError)
	}
	entry, configured, err := service.effective(ctx, role.Provider)
	if err != nil {
		return nil, *role, err
	}
	if !configured {
		return nil, *role, fmt.Errorf("%w: provider %q is not configured", protocol.ErrProviderError, role.Provider)
	}
	model, err := llmadapter.BuildEmbeddingModel(llmadapter.ClientSpec{
		Provider: llmadapter.Provider(role.Provider), Model: role.Model,
		APIKey: entry.APIKey, BaseURL: entry.BaseURL,
	})
	return model, *role, err
}

func (service *Service) DefaultSelection(ctx context.Context) (string, string, error) {
	for _, providerID := range llmadapter.SupportedProviders() {
		_, configured, err := service.effective(ctx, string(providerID))
		if err != nil {
			return "", "", err
		}
		model := providerID.DefaultModel()
		if configured && model != "" {
			return string(providerID), model, nil
		}
	}
	return "", "", fmt.Errorf("%w: configure a model provider before starting a run", protocol.ErrProviderError)
}

func (service *Service) effective(ctx context.Context, id string) (provider.Provider, bool, error) {
	entry, found, err := service.store.GetProvider(ctx, id)
	if err != nil {
		return provider.Provider{}, false, err
	}
	if !found {
		entry.ID = id
	}
	if entry.APIKey != "" {
		entry.KeySource = provider.KeyStored
		return entry, true, nil
	}
	if key := service.environment[id]; key != "" {
		entry.APIKey = key
		entry.KeySource = provider.KeyEnv
		return entry, true, nil
	}
	// Local Ollama can be usable without a credential.
	if llmadapter.Provider(id) == llmadapter.ProviderOllama {
		return entry, true, nil
	}
	return entry, false, nil
}

func providerPatch(request protocol.UpdateProviderRequest) (provider.Patch, error) {
	baseURL, err := textChange(request.BaseURL)
	if err != nil {
		return provider.Patch{}, fmt.Errorf("%w: baseUrl: %v", protocol.ErrInvalidParams, err)
	}
	apiKey, err := textChange(request.APIKey)
	if err != nil {
		return provider.Patch{}, fmt.Errorf("%w: apiKey: %v", protocol.ErrInvalidParams, err)
	}
	return provider.Patch{BaseURL: baseURL, APIKey: apiKey}, nil
}

func textChange(change *protocol.ProviderConfigChange) (provider.TextChange, error) {
	if change == nil {
		return provider.TextChange{}, nil
	}
	switch change.Type {
	case protocol.ProviderConfigClear:
		return provider.TextChange{Present: true, Clear: true}, nil
	case protocol.ProviderConfigSet:
		if change.Value == nil || strings.TrimSpace(*change.Value) == "" {
			return provider.TextChange{}, errors.New("set requires a non-empty value")
		}
		return provider.TextChange{Present: true, Value: *change.Value}, nil
	default:
		return provider.TextChange{}, errors.New("unknown change type")
	}
}

func presentProvider(id llmadapter.Provider, entry provider.Provider) protocol.Provider {
	keySource := protocol.ProviderKeySource(entry.KeySource)
	return protocol.Provider{
		ID: string(id), BaseURL: entry.BaseURL, APIKeyMasked: mask(entry.APIKey), KeySource: keySource,
		RequiresBaseURL: id.RequiresBaseURL(), EmbeddingCapable: id.EmbeddingCapable(),
		DefaultEmbeddingModel: id.DefaultEmbeddingModel(),
	}
}

func presentModel(providerID string, entry catalog.Model) protocol.Model {
	capabilities := &protocol.ModelCapabilities{
		Reasoning: entry.Reasoning.Supported, ReasoningLevels: slices.Clone(entry.Reasoning.Levels),
		ReasoningDefaultLevel: entry.Reasoning.DefaultLevel,
		Multimodal: entry.Modalities.AcceptsInput(catalog.ModalityImage),
		InputModalities: modalities(entry.Modalities.Input), OutputModalities: modalities(entry.Modalities.Output),
		ToolUse: entry.ToolCall, StructuredOutput: entry.StructuredOutput,
	}
	model := protocol.Model{
		ID: entry.ID, Provider: providerID, DisplayName: entry.DisplayName,
		ContextWindow: int(entry.Limits.ContextWindow), MaxInputTokens: int(entry.Limits.MaxInputTokens),
		MaxOutputTokens: int(entry.Limits.MaxOutputTokens), Deprecated: entry.Deprecated,
		Capabilities: capabilities,
	}
	if !entry.KnowledgeCutoff.IsZero() {
		model.KnowledgeCutoff = entry.KnowledgeCutoff.Format("2006-01-02")
	}
	if len(entry.Pricing) > 0 {
		pricing := entry.Pricing[0]
		model.Pricing = &protocol.ModelPricing{
			InputUSDPerMillionTokens: pricing.InputPer1M, OutputUSDPerMillionTokens: pricing.OutputPer1M,
			CacheReadUSDPerMillionTokens: pricing.CacheReadPer1M,
			CacheWriteUSDPerMillionTokens: pricing.CacheWritePer1M,
		}
	}
	return model
}

func modalities(values []catalog.Modality) []protocol.Modality {
	result := make([]protocol.Modality, len(values))
	for index, value := range values {
		result[index] = protocol.Modality(value)
	}
	return result
}

func mask(value string) string {
	if value == "" {
		return ""
	}
	if len(value) < 9 {
		return "****"
	}
	return value[:2] + "****" + value[len(value)-2:]
}

func probe(ctx context.Context, providerID llmadapter.Provider, entry provider.Provider) error {
	if providerID.ProbeModels() {
		baseURL := entry.BaseURL
		if baseURL == "" {
			baseURL = providerID.DefaultBaseURL()
		}
		models, err := llmadapter.ListRemoteModels(ctx, baseURL, entry.APIKey)
		if err != nil {
			return err
		}
		if len(models) == 0 {
			return errors.New("provider returned no models")
		}
		return nil
	}
	client, err := llmadapter.BuildClient(llmadapter.ClientSpec{
		Provider: providerID, Model: providerID.DefaultModel(), APIKey: entry.APIKey, BaseURL: entry.BaseURL,
	})
	if err != nil {
		return err
	}
	maxTokens := int64(1)
	_, err = client.Call(ctx, &chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("ping"))},
		Options: chat.Options{MaxTokens: &maxTokens},
	})
	return err
}
