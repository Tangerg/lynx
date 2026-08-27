// Package modelcatalog adapts provider infrastructure and static catalog data
// to the application/models ports.
package modelcatalog

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/chat"
	catalog "github.com/Tangerg/scope/models/catalog"

	modelsapp "github.com/Tangerg/scope/app/runtime/internal/application/models"
	"github.com/Tangerg/scope/app/runtime/internal/domain/provider"
	"github.com/Tangerg/scope/app/runtime/internal/infra/llm"
)

// Capabilities implements the three model-configuration ports consumed by
// model use cases: static catalog lookup, credential probing, and remote listing.
// Keeping them together is justified because they share one provider
// integration boundary.
type Capabilities struct{}

func (Capabilities) Supported() []modelsapp.ProviderMetadata {
	supported := llm.SupportedProviders()
	out := make([]modelsapp.ProviderMetadata, 0, len(supported))
	for _, value := range supported {
		out = append(out, providerMetadata(value))
	}
	return out
}

func (Capabilities) Metadata(id string) (modelsapp.ProviderMetadata, bool) {
	value := llm.Provider(id)
	if !value.IsSupported() {
		return modelsapp.ProviderMetadata{}, false
	}
	return providerMetadata(value), true
}

func (Capabilities) Models(providerID string) []modelsapp.Model {
	entries := catalog.Default.Models(providerID)
	out := make([]modelsapp.Model, 0, len(entries))
	for _, entry := range entries {
		out = append(out, catalogModel(providerID, entry))
	}
	return out
}

func (Capabilities) LookupModel(providerID, modelID string) (modelsapp.Model, bool) {
	entry, ok := catalog.Default.Lookup(providerID, modelID)
	if !ok {
		return modelsapp.Model{}, false
	}
	return catalogModel(providerID, entry), true
}

func (Capabilities) Probe(ctx context.Context, entry provider.Provider) error {
	providerID := llm.Provider(entry.ID)
	if providerID.ProbeModels() {
		models, err := remoteModelIDs(ctx, entry)
		if err != nil {
			return err
		}
		if len(models) == 0 {
			return fmt.Errorf("modelcatalog: provider %q advertised no models", entry.ID)
		}
		return nil
	}
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: providerID, Model: providerID.DefaultModel(), APIKey: entry.APIKey, BaseURL: entry.BaseURL,
	})
	if err != nil {
		return err
	}
	maxTokens := int64(1)
	_, err = client.Call(ctx, &chat.Request{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("ping"))}, Options: chat.Options{MaxTokens: &maxTokens},
	})
	return err
}

func (Capabilities) ListModels(ctx context.Context, entry provider.Provider) ([]string, error) {
	return remoteModelIDs(ctx, entry)
}

func remoteModelIDs(ctx context.Context, entry provider.Provider) ([]string, error) {
	value := llm.Provider(entry.ID)
	baseURL := entry.BaseURL
	if baseURL == "" {
		baseURL = value.DefaultBaseURL()
	}
	if baseURL == "" {
		return nil, nil
	}
	return llm.ListRemoteModels(ctx, baseURL, entry.APIKey)
}

func providerMetadata(value llm.Provider) modelsapp.ProviderMetadata {
	return modelsapp.ProviderMetadata{
		ID: string(value), RequiresBaseURL: value.RequiresBaseURL(), EmbeddingCapable: value.EmbeddingCapable(),
		DefaultEmbeddingModel: value.DefaultEmbeddingModel(), ProbeModels: value.ProbeModels(),
	}
}

func catalogModel(providerID string, entry catalog.Model) modelsapp.Model {
	details := &modelsapp.Details{
		DisplayName: entry.DisplayName, ContextWindow: int(entry.Limits.ContextWindow), MaxInputTokens: int(entry.Limits.MaxInputTokens),
		MaxOutputTokens: int(entry.Limits.MaxOutputTokens), KnowledgeCutoff: entry.KnowledgeCutoff, Deprecated: entry.Deprecated,
		Reasoning: entry.Reasoning.Supported, ReasoningLevels: slices.Clone(entry.Reasoning.Levels), ReasoningDefault: entry.Reasoning.DefaultLevel,
		Multimodal: entry.Modalities.AcceptsInput(catalog.ModalityImage), InputModalities: catalogModalities(entry.Modalities.Input),
		OutputModalities: catalogModalities(entry.Modalities.Output), ToolUse: entry.ToolCall, StructuredOutput: entry.StructuredOutput,
	}
	if len(entry.Pricing) > 0 {
		primary := entry.Pricing[0]
		details.Pricing = &modelsapp.Pricing{
			InputPerMillion: primary.InputPer1M, OutputPerMillion: primary.OutputPer1M,
			CacheReadPerMillion: primary.CacheReadPer1M, CacheWritePerMillion: primary.CacheWritePer1M,
		}
	}
	return modelsapp.Model{ID: entry.ID, Provider: providerID, Details: details}
}

func catalogModalities(values []catalog.Modality) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = string(value)
	}
	return out
}
