package llm

import (
	"fmt"

	openaiopt "github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/embedding"

	"github.com/Tangerg/lynx/models/alibaba"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/mistral"
	"github.com/Tangerg/lynx/models/ollama"
	"github.com/Tangerg/lynx/models/openai"
	"github.com/Tangerg/lynx/models/zhipu"
)

// embeddingBuildFunc constructs an embedding adapter for one (key, model, baseURL).
type embeddingBuildFunc func(spec ClientSpec, opts *embedding.Options) (embedding.Model, error)

type embeddingEntry struct {
	defaultModel string
	build        embeddingBuildFunc
}

// embeddingProviderInfo is the embedding counterpart of [providerInfo] — the
// providers Lyra already imports that ALSO offer an embeddings endpoint.
// Anthropic is intentionally absent (it has no embeddings API); local Ollama
// gives a key-free embedding path for anyone, including Anthropic-only users.
// The credential (key + base URL) comes from the same provider registry the
// chat clients use — an embedding role names a (provider, model), nothing more.
var embeddingProviderInfo = map[Provider]embeddingEntry{
	ProviderOpenAI: {defaultModel: "text-embedding-3-small", build: func(s ClientSpec, o *embedding.Options) (embedding.Model, error) {
		var reqOpts []openaiopt.RequestOption
		if s.BaseURL != "" {
			reqOpts = append(reqOpts, openaiopt.WithBaseURL(s.BaseURL))
		}
		return openai.NewEmbeddingModel(openai.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, RequestOptions: reqOpts})
	}},
	ProviderAzureOpenAI: {build: func(s ClientSpec, o *embedding.Options) (embedding.Model, error) {
		return azureopenai.NewEmbeddingModel(azureopenai.EmbeddingModelConfig{APIKey: s.APIKey, Endpoint: s.BaseURL, DefaultOptions: o})
	}},
	ProviderGoogle: {defaultModel: "text-embedding-004", build: func(s ClientSpec, o *embedding.Options) (embedding.Model, error) {
		return google.NewEmbeddingModel(google.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderMistral: {defaultModel: "mistral-embed", build: func(s ClientSpec, o *embedding.Options) (embedding.Model, error) {
		return mistral.NewEmbeddingModel(mistral.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderOllama: {defaultModel: "nomic-embed-text", build: func(s ClientSpec, o *embedding.Options) (embedding.Model, error) {
		// Ollama is a keyless local daemon — no APIKey field.
		return ollama.NewEmbeddingModel(ollama.EmbeddingModelConfig{DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderZhipu: {defaultModel: "embedding-3", build: func(s ClientSpec, o *embedding.Options) (embedding.Model, error) {
		return zhipu.NewEmbeddingModel(zhipu.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderAlibaba: {defaultModel: "text-embedding-v3", build: func(s ClientSpec, o *embedding.Options) (embedding.Model, error) {
		return alibaba.NewEmbeddingModel(alibaba.EmbeddingModelConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
}

// EmbeddingCapable reports whether p has an embeddings adapter.
func EmbeddingCapable(p Provider) bool {
	_, ok := embeddingProviderInfo[p]
	return ok
}

// DefaultEmbeddingModel returns p's default embedding model id, or "" when the
// id is always user-supplied (Azure deployment names).
func DefaultEmbeddingModel(p Provider) string {
	return embeddingProviderInfo[p].defaultModel
}

// BuildEmbeddingModel wires an embedding.Model for one provider+model from
// [embeddingProviderInfo], threading the api key + optional base URL (Azure's
// per-resource endpoint, Ollama's localhost, an OpenAI-compatible gateway).
func BuildEmbeddingModel(spec ClientSpec) (embedding.Model, error) {
	entry, ok := embeddingProviderInfo[spec.Provider]
	if !ok {
		return nil, fmt.Errorf("llm: provider %q has no embeddings adapter", spec.Provider)
	}
	opts, err := embedding.NewOptions(spec.Model)
	if err != nil {
		return nil, fmt.Errorf("llm: embedding options for %q: %w", spec.Model, err)
	}
	m, err := entry.build(spec, opts)
	if err != nil {
		return nil, fmt.Errorf("llm: build %s embedding model: %w", spec.Provider, err)
	}
	return m, nil
}
