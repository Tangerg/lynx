// Package providerflow owns provider credentials, live probes and model-role
// selection. Vendor wire details remain in llmadapter.
package providerflow

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelselection"
	"github.com/Tangerg/lynx/app2/runtime/domain/provider"
	"github.com/Tangerg/lynx/app2/runtime/llmadapter"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/embedding"
	catalog "github.com/Tangerg/lynx/models/catalog"
)

type Store interface {
	GetProvider(context.Context, string) (provider.Configuration, bool, error)
	SaveProvider(context.Context, provider.Configuration, uint64) error
	GetModelRole(context.Context, modelselection.Role) (modelselection.Selection, bool, error)
	PutModelRole(context.Context, modelselection.Role, modelselection.Selection) error
}

type Service struct {
	store       Store
	environment map[string]string
}

type ProviderUpdate struct {
	Provider *protocol.Provider
	Changed  bool
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

func (service *Service) Update(ctx context.Context, request protocol.UpdateProviderRequest) (ProviderUpdate, error) {
	providerID := llmadapter.Provider(request.Provider)
	if !providerID.IsSupported() {
		return ProviderUpdate{}, fmt.Errorf("%w: unsupported provider %q", protocol.ErrInvalidParams, request.Provider)
	}
	if request.APIKey == nil && request.BaseURL == nil {
		return ProviderUpdate{}, fmt.Errorf("%w: provider update has no changes", protocol.ErrInvalidParams)
	}
	patch, err := providerPatch(request)
	if err != nil {
		return ProviderUpdate{}, err
	}
	for range 8 {
		stored, found, err := service.store.GetProvider(ctx, request.Provider)
		if err != nil {
			return ProviderUpdate{}, err
		}
		previousRevision := uint64(0)
		if found {
			previousRevision = stored.Revision()
		} else {
			stored, err = provider.New(request.Provider)
			if err != nil {
				return ProviderUpdate{}, err
			}
		}
		if !stored.Apply(patch) {
			effective := service.projectEffective(stored)
			presented := presentProvider(providerID, effective)
			return ProviderUpdate{Provider: &presented}, nil
		}
		if err := stored.Validate(providerID.RequiresBaseURL()); err != nil {
			return ProviderUpdate{}, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
		}
		if err := service.store.SaveProvider(ctx, stored, previousRevision); err != nil {
			if errors.Is(err, provider.ErrRevisionConflict) {
				continue
			}
			return ProviderUpdate{}, err
		}
		effective := service.projectEffective(stored)
		presented := presentProvider(providerID, effective)
		return ProviderUpdate{Provider: &presented, Changed: true}, nil
	}
	return ProviderUpdate{}, fmt.Errorf("%w: provider %q remained busy after concurrent updates", protocol.ErrProviderError, request.Provider)
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
		return &protocol.ProviderTestResult{Error: &protocol.ProblemData{
			Type:   protocol.ProblemProviderNotConfigured,
			Detail: "Configure the provider before testing it.",
		}}, nil
	}
	if err := probe(ctx, providerID, entry); err != nil {
		return &protocol.ProviderTestResult{Error: &protocol.ProblemData{
			Type:   protocol.ProblemProviderTestFailed,
			Detail: probeDetail(err, entry.apiKey),
		}}, nil
	}
	return &protocol.ProviderTestResult{OK: true}, nil
}

func (service *Service) Models(ctx context.Context, request protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	if request.Provider == "" {
		return protocol.NewPage([]protocol.Model{}), nil
	}
	providerID := llmadapter.Provider(request.Provider)
	if !providerID.IsSupported() {
		return nil, fmt.Errorf("%w: unsupported provider %q", protocol.ErrInvalidParams, request.Provider)
	}
	if providerID.ProbeModels() {
		entry, configured, err := service.effective(ctx, request.Provider)
		if err != nil {
			return nil, err
		}
		if !configured {
			return nil, fmt.Errorf("%w: provider %q is not configured", protocol.ErrProviderError, request.Provider)
		}
		baseURL := entry.configuration.BaseURL()
		if baseURL == "" {
			baseURL = providerID.DefaultBaseURL()
		}
		if baseURL == "" {
			return nil, fmt.Errorf("%w: provider %q has no model discovery endpoint", protocol.ErrProviderError, request.Provider)
		}
		ids, err := llmadapter.ListRemoteModels(ctx, baseURL, entry.apiKey)
		if err != nil {
			return nil, fmt.Errorf("%w: discover models for %q: %v", protocol.ErrProviderError, request.Provider, err)
		}
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
	entries := catalog.Models(request.Provider)
	data := make([]protocol.Model, 0, len(entries))
	for _, entry := range entries {
		data = append(data, presentModel(request.Provider, entry))
	}
	return protocol.NewPage(data), nil
}

func (service *Service) UtilityRole(ctx context.Context) (*protocol.UtilityRole, error) {
	selection, _, err := service.store.GetModelRole(ctx, modelselection.RoleUtility)
	if err != nil {
		return nil, err
	}
	return &protocol.UtilityRole{Provider: selection.Provider(), Model: selection.Model()}, nil
}

func (service *Service) SetUtilityRole(ctx context.Context, role protocol.UtilityRole) (*protocol.UtilityRole, bool, error) {
	selection, changed, err := service.setRole(ctx, modelselection.RoleUtility, role.Provider, role.Model)
	if err != nil {
		return nil, false, err
	}
	return &protocol.UtilityRole{Provider: selection.Provider(), Model: selection.Model()}, changed, nil
}

func (service *Service) EmbeddingRole(ctx context.Context) (*protocol.EmbeddingRole, error) {
	selection, _, err := service.store.GetModelRole(ctx, modelselection.RoleEmbedding)
	if err != nil {
		return nil, err
	}
	return &protocol.EmbeddingRole{Provider: selection.Provider(), Model: selection.Model()}, nil
}

func (service *Service) SetEmbeddingRole(ctx context.Context, role protocol.EmbeddingRole) (*protocol.EmbeddingRole, bool, error) {
	selection, changed, err := service.setRole(ctx, modelselection.RoleEmbedding, role.Provider, role.Model)
	if err != nil {
		return nil, false, err
	}
	return &protocol.EmbeddingRole{Provider: selection.Provider(), Model: selection.Model()}, changed, nil
}

func (service *Service) setRole(ctx context.Context, role modelselection.Role, providerID, model string) (modelselection.Selection, bool, error) {
	selection, err := modelselection.New(providerID, model)
	if err != nil {
		return modelselection.Selection{}, false, fmt.Errorf("%w: %v", protocol.ErrInvalidParams, err)
	}
	if err := service.validateRole(ctx, role, selection); err != nil {
		return modelselection.Selection{}, false, err
	}
	current, found, err := service.store.GetModelRole(ctx, role)
	if err != nil {
		return modelselection.Selection{}, false, err
	}
	if (!found && selection.Empty()) || (found && current == selection) {
		return current, false, nil
	}
	if err := service.store.PutModelRole(ctx, role, selection); err != nil {
		return modelselection.Selection{}, false, err
	}
	return selection, true, nil
}

func (service *Service) validateRole(ctx context.Context, role modelselection.Role, selection modelselection.Selection) error {
	if selection.Empty() {
		return nil
	}
	providerID, model := selection.Provider(), selection.Model()
	metadata := llmadapter.Provider(providerID)
	embedding := role == modelselection.RoleEmbedding
	if !metadata.IsSupported() || (embedding && !metadata.EmbeddingCapable()) {
		return fmt.Errorf("%w: provider %q cannot serve this role", protocol.ErrInvalidParams, providerID)
	}
	entry, configured, err := service.effective(ctx, providerID)
	if err != nil {
		return err
	}
	if !configured {
		return fmt.Errorf("%w: provider %q is not configured", protocol.ErrProviderError, providerID)
	}
	if embedding {
		_, err := llmadapter.BuildEmbeddingModel(llmadapter.ClientSpec{
			Provider: metadata, Model: model, APIKey: entry.apiKey, BaseURL: entry.configuration.BaseURL(),
		})
		if err != nil {
			return fmt.Errorf("%w: invalid embedding selection: %v", protocol.ErrInvalidParams, err)
		}
		return nil
	}
	if metadata.ProbeModels() {
		page, err := service.Models(ctx, protocol.ListModelsRequest{Provider: providerID})
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(page.Data, func(candidate protocol.Model) bool { return candidate.ID == model }) {
			return fmt.Errorf("%w: model %q is not available from provider %q", protocol.ErrInvalidParams, model, providerID)
		}
		return nil
	}
	if _, found := catalog.Lookup(providerID, model); !found {
		return fmt.Errorf("%w: model %q is not in provider %q's catalog", protocol.ErrInvalidParams, model, providerID)
	}
	return nil
}

func (service *Service) ResolveClient(ctx context.Context, providerID, model string) (*chatclient.Client, error) {
	selection, err := modelselection.New(providerID, model)
	if err != nil || selection.Empty() || !llmadapter.Provider(providerID).IsSupported() {
		return nil, fmt.Errorf("%w: invalid provider/model selection", protocol.ErrProviderError)
	}
	entry, configured, err := service.effective(ctx, providerID)
	if err != nil {
		return nil, err
	}
	if !configured {
		return nil, fmt.Errorf("%w: provider %q is not configured", protocol.ErrProviderError, providerID)
	}
	return llmadapter.BuildClient(llmadapter.ClientSpec{
		Provider: llmadapter.Provider(providerID), Model: model,
		APIKey: entry.apiKey, BaseURL: entry.configuration.BaseURL(),
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
		APIKey: entry.apiKey, BaseURL: entry.configuration.BaseURL(),
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

type effectiveProvider struct {
	configuration provider.Configuration
	apiKey        string
	keySource     protocol.ProviderKeySource
}

func (service *Service) effective(ctx context.Context, id string) (effectiveProvider, bool, error) {
	entry, found, err := service.store.GetProvider(ctx, id)
	if err != nil {
		return effectiveProvider{}, false, err
	}
	if !found {
		entry, err = provider.New(id)
		if err != nil {
			return effectiveProvider{}, false, err
		}
	}
	effective := service.projectEffective(entry)
	if effective.apiKey != "" {
		return effective, true, nil
	}
	// Local Ollama can be usable without a credential.
	if llmadapter.Provider(id) == llmadapter.ProviderOllama {
		return effective, true, nil
	}
	return effective, false, nil
}

func (service *Service) projectEffective(configuration provider.Configuration) effectiveProvider {
	if configuration.APIKey() != "" {
		return effectiveProvider{
			configuration: configuration, apiKey: configuration.APIKey(),
			keySource: protocol.ProviderKeySourceStored,
		}
	}
	if key := service.environment[configuration.ID()]; key != "" {
		return effectiveProvider{
			configuration: configuration, apiKey: key,
			keySource: protocol.ProviderKeySourceEnv,
		}
	}
	return effectiveProvider{configuration: configuration}
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

func presentProvider(id llmadapter.Provider, entry effectiveProvider) protocol.Provider {
	return protocol.Provider{
		ID: string(id), BaseURL: entry.configuration.BaseURL(), APIKeyMasked: mask(entry.apiKey), KeySource: entry.keySource,
		RequiresBaseURL: id.RequiresBaseURL(), EmbeddingCapable: id.EmbeddingCapable(),
		DefaultEmbeddingModel: id.DefaultEmbeddingModel(),
	}
}

func presentModel(providerID string, entry catalog.Model) protocol.Model {
	capabilities := &protocol.ModelCapabilities{
		Reasoning: entry.Reasoning.Supported, ReasoningLevels: slices.Clone(entry.Reasoning.Levels),
		ReasoningDefaultLevel: entry.Reasoning.DefaultLevel,
		Multimodal:            entry.Modalities.AcceptsInput(catalog.ModalityImage),
		InputModalities:       modalities(entry.Modalities.Input), OutputModalities: modalities(entry.Modalities.Output),
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
			CacheReadUSDPerMillionTokens:  pricing.CacheReadPer1M,
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

func probeDetail(err error, secret string) string {
	detail := strings.TrimSpace(err.Error())
	if secret != "" {
		detail = strings.ReplaceAll(detail, secret, "[redacted]")
	}
	characters := []rune(detail)
	if len(characters) > 512 {
		detail = string(characters[:512])
	}
	return detail
}

func probe(ctx context.Context, providerID llmadapter.Provider, entry effectiveProvider) error {
	if providerID.ProbeModels() {
		baseURL := entry.configuration.BaseURL()
		if baseURL == "" {
			baseURL = providerID.DefaultBaseURL()
		}
		models, err := llmadapter.ListRemoteModels(ctx, baseURL, entry.apiKey)
		if err != nil {
			return err
		}
		if len(models) == 0 {
			return errors.New("provider returned no models")
		}
		return nil
	}
	client, err := llmadapter.BuildClient(llmadapter.ClientSpec{
		Provider: providerID, Model: providerID.DefaultModel(), APIKey: entry.apiKey, BaseURL: entry.configuration.BaseURL(),
	})
	if err != nil {
		return err
	}
	maxTokens := int64(1)
	_, err = client.Call(ctx, &chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("ping"))},
		Options:  chat.Options{MaxTokens: &maxTokens},
	})
	return err
}
