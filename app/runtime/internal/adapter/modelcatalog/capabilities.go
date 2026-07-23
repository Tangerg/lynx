// Package modelcatalog adapts provider infrastructure and static catalog data
// to the application/models ports. Bootstrap only constructs this adapter.
package modelcatalog

import (
	"context"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	catalog "github.com/Tangerg/lynx/models/catalog"

	modelsapp "github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

// Capabilities implements the three model-configuration ports consumed by
// Application: static catalog lookup, credential probing, and remote listing.
// Keeping them together is justified because they share the same provider
// infrastructure boundary and have one composition-root construction point.
type Capabilities struct{}

func (Capabilities) Supported() []provider.Metadata {
	supported := llm.SupportedProviders()
	out := make([]provider.Metadata, 0, len(supported))
	for _, value := range supported {
		out = append(out, providerMetadata(value))
	}
	return out
}

func (Capabilities) Metadata(id string) (provider.Metadata, bool) {
	value := llm.Provider(id)
	if !value.IsSupported() {
		return provider.Metadata{}, false
	}
	return providerMetadata(value), true
}

func (Capabilities) Models(providerID string) []modelsapp.Model {
	entries := catalog.Models(providerID)
	out := make([]modelsapp.Model, 0, len(entries))
	for _, entry := range entries {
		out = append(out, catalogModel(providerID, entry))
	}
	return out
}

func (Capabilities) LookupModel(providerID, modelID string) (modelsapp.Model, bool) {
	entry, ok := catalog.Lookup(providerID, modelID)
	if !ok {
		return modelsapp.Model{}, false
	}
	return catalogModel(providerID, entry), true
}

func (Capabilities) Probe(ctx context.Context, entry provider.Provider) error {
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(entry.ID), Model: llm.Provider(entry.ID).DefaultModel(), APIKey: entry.APIKey, BaseURL: entry.BaseURL,
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

func providerMetadata(value llm.Provider) provider.Metadata {
	return provider.Metadata{
		ID: string(value), RequiresBaseURL: value.RequiresBaseURL(), EmbeddingCapable: value.EmbeddingCapable(),
		DefaultEmbeddingModel: value.DefaultEmbeddingModel(), ProbeModels: value.ProbeModels(),
	}
}

func catalogModel(providerID string, entry catalog.Model) modelsapp.Model {
	details := &modelsapp.ModelDetails{
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
