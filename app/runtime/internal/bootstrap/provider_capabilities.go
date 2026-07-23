package bootstrap

import (
	"context"
	"slices"

	"github.com/Tangerg/lynx/core/chat"
	modelcatalog "github.com/Tangerg/lynx/models/catalog"

	modelsapp "github.com/Tangerg/lynx/app/runtime/internal/application/models"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/provider"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

// providerCatalog projects infrastructure provider adapters and the static model
// catalog into application values. Only the composition root reads those sources,
// so this projection lives here rather than in the application ring.
type providerCatalog struct{}

func (providerCatalog) Supported() []provider.Metadata {
	supported := llm.SupportedProviders()
	out := make([]provider.Metadata, 0, len(supported))
	for _, p := range supported {
		out = append(out, providerMetadata(p))
	}
	return out
}

func (providerCatalog) Metadata(id string) (provider.Metadata, bool) {
	if !llm.Provider(id).IsSupported() {
		return provider.Metadata{}, false
	}
	return providerMetadata(llm.Provider(id)), true
}

func (providerCatalog) Models(providerID string) []modelsapp.Model {
	entries := modelcatalog.Models(providerID)
	out := make([]modelsapp.Model, 0, len(entries))
	for _, entry := range entries {
		out = append(out, catalogModel(providerID, entry))
	}
	return out
}

func (providerCatalog) LookupModel(providerID, modelID string) (modelsapp.Model, bool) {
	entry, ok := modelcatalog.Lookup(providerID, modelID)
	if !ok {
		return modelsapp.Model{}, false
	}
	return catalogModel(providerID, entry), true
}

func providerMetadata(p llm.Provider) provider.Metadata {
	return provider.Metadata{
		ID:                    string(p),
		RequiresBaseURL:       p.RequiresBaseURL(),
		EmbeddingCapable:      p.EmbeddingCapable(),
		DefaultEmbeddingModel: p.DefaultEmbeddingModel(),
		ProbeModels:           p.ProbeModels(),
	}
}

func catalogModel(providerID string, entry modelcatalog.Model) modelsapp.Model {
	details := &modelsapp.ModelDetails{
		DisplayName:      entry.DisplayName,
		ContextWindow:    int(entry.Limits.ContextWindow),
		MaxInputTokens:   int(entry.Limits.MaxInputTokens),
		MaxOutputTokens:  int(entry.Limits.MaxOutputTokens),
		KnowledgeCutoff:  entry.KnowledgeCutoff,
		Deprecated:       entry.Deprecated,
		Reasoning:        entry.Reasoning.Supported,
		ReasoningLevels:  slices.Clone(entry.Reasoning.Levels),
		ReasoningDefault: entry.Reasoning.DefaultLevel,
		Multimodal:       entry.Modalities.AcceptsInput(modelcatalog.ModalityImage),
		InputModalities:  catalogModalities(entry.Modalities.Input),
		OutputModalities: catalogModalities(entry.Modalities.Output),
		ToolUse:          entry.ToolCall,
		StructuredOutput: entry.StructuredOutput,
	}
	if len(entry.Pricing) > 0 {
		primary := entry.Pricing[0]
		details.Pricing = &modelsapp.Pricing{
			InputPerMillion:      primary.InputPer1M,
			OutputPerMillion:     primary.OutputPer1M,
			CacheReadPerMillion:  primary.CacheReadPer1M,
			CacheWritePerMillion: primary.CacheWritePer1M,
		}
	}
	return modelsapp.Model{ID: entry.ID, Provider: providerID, Details: details}
}

func catalogModalities(in []modelcatalog.Modality) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, modality := range in {
		out[i] = string(modality)
	}
	return out
}

// providerProber validates a provider's credentials by building its default-model
// client and issuing one minimal (max_tokens=1) request — the cheapest call that
// proves the key + endpoint work. Lives here because the composition root owns
// client construction against the infra provider adapters. Returns the provider
// error verbatim so the caller can surface it inline.
type providerProber struct{}

func (providerProber) Probe(ctx context.Context, entry provider.Provider) error {
	client, err := llm.BuildClient(llm.ClientSpec{
		Provider: llm.Provider(entry.ID),
		Model:    llm.Provider(entry.ID).DefaultModel(),
		APIKey:   entry.APIKey,
		BaseURL:  entry.BaseURL,
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

// providerModelLister discovers a provider's live model list by probing its
// OpenAI-compatible /v1/models endpoint. It resolves the endpoint from the
// configured base URL, falling back to the provider's built-in default (the
// local Ollama daemon); a provider with neither has no endpoint to probe.
// Lives here because the composition root owns endpoint resolution against the
// infra provider table.
type providerModelLister struct{}

func (providerModelLister) ListModels(ctx context.Context, entry provider.Provider) ([]string, error) {
	p := llm.Provider(entry.ID)
	baseURL := entry.BaseURL
	if baseURL == "" {
		baseURL = p.DefaultBaseURL()
	}
	if baseURL == "" {
		return nil, nil
	}
	return llm.ListRemoteModels(ctx, baseURL, entry.APIKey)
}
