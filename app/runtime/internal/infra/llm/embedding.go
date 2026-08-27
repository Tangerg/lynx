package llm

import (
	"fmt"

	"github.com/Tangerg/scope/core/embedding"

	"github.com/Tangerg/scope/models/alibaba"
	"github.com/Tangerg/scope/models/azureopenai"
	"github.com/Tangerg/scope/models/google"
	"github.com/Tangerg/scope/models/mistral"
	openaimodel "github.com/Tangerg/scope/models/openai"
	"github.com/Tangerg/scope/models/zhipu"
)

const defaultOpenAIEmbeddingModel = "text-embedding-3-small"

// embeddingBuildFunc constructs an embedding adapter for one (key, model, baseURL).
type embeddingBuildFunc func(spec ClientSpec, opts embedding.Options) (embedding.Model, error)

type embeddingProviderProfile struct {
	defaultModel string
	build        embeddingBuildFunc
}

// embeddingProviderCatalog is the embedding counterpart of [chatProviderCatalog] — the
// providers Lyra already imports that ALSO offer an embeddings endpoint.
// Anthropic is intentionally absent (it has no embeddings API); local Ollama
// gives a key-free embedding path for anyone, including Anthropic-only users.
// The credential (key + base URL) comes from the same provider registry the
// chat clients use — an embedding role names a (provider, model), nothing more.
var embeddingProviderCatalog = map[Provider]embeddingProviderProfile{
	ProviderOpenAI: {defaultModel: defaultOpenAIEmbeddingModel, build: func(s ClientSpec, o embedding.Options) (embedding.Model, error) {
		return openaimodel.NewEmbeddingModel(openaimodel.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderAzureOpenAI: {build: func(s ClientSpec, o embedding.Options) (embedding.Model, error) {
		return azureopenai.NewEmbeddingModel(azureopenai.EmbeddingModelConfig{APIKey: s.APIKey, BaseURL: s.BaseURL, DefaultOptions: o})
	}},
	ProviderGoogle: {defaultModel: google.ModelGeminiEmbedding2, build: func(s ClientSpec, o embedding.Options) (embedding.Model, error) {
		return google.NewEmbeddingModel(google.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderMistral: {defaultModel: mistral.ModelEmbed, build: func(s ClientSpec, o embedding.Options) (embedding.Model, error) {
		return mistral.NewEmbeddingModel(mistral.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderOllama: {defaultModel: "nomic-embed-text", build: buildOllamaEmbeddingModel},
	ProviderZhipu: {defaultModel: zhipu.ModelEmbedding3, build: func(s ClientSpec, o embedding.Options) (embedding.Model, error) {
		return zhipu.NewEmbeddingModel(zhipu.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderAlibaba: {defaultModel: alibaba.ModelEmbeddingV4, build: func(s ClientSpec, o embedding.Options) (embedding.Model, error) {
		return alibaba.NewEmbeddingModel(alibaba.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
}

// EmbeddingCapable reports whether p has an embeddings adapter.
func (p Provider) EmbeddingCapable() bool {
	_, ok := embeddingProviderCatalog[p]
	return ok
}

// DefaultEmbeddingModel returns p's default embedding model id, or "" when the
// id is always user-supplied (Azure deployment names).
func (p Provider) DefaultEmbeddingModel() string {
	return embeddingProviderCatalog[p].defaultModel
}

// BuildEmbeddingModel wires an embedding.Model for one provider+model from
// [embeddingProviderCatalog], threading the api key + optional base URL (Azure's
// per-resource endpoint, Ollama's localhost, an OpenAI-compatible gateway).
func BuildEmbeddingModel(spec ClientSpec) (embedding.Model, error) {
	profile, ok := embeddingProviderCatalog[spec.Provider]
	if !ok {
		return nil, fmt.Errorf("llm: provider %q has no embeddings adapter", spec.Provider)
	}
	opts, err := embedding.NewOptions(spec.Model)
	if err != nil {
		return nil, fmt.Errorf("llm: embedding options for %q: %w", spec.Model, err)
	}
	model, err := profile.build(spec, opts)
	if err != nil {
		return nil, fmt.Errorf("llm: build %s embedding model: %w", spec.Provider, err)
	}
	return model, nil
}
