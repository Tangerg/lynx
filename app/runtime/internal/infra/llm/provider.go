// Package llm is Lyra's catalog of supported LLM providers and the
// construction of a chat client for one. It owns the static provider table
// (which vendors Lyra can talk to, each one's default model, key env var, and
// wire adapter) and [BuildClient], which wires a vendor's lynx model adapter
// into a *chat.Client. It is pure infrastructure: it wraps the external model
// SDKs and depends on no inner ring.
//
// The runtime-mutable credential registry (a provider's configured key + base
// URL) is a separate concern — see internal/domain/provider. This package is
// "what providers exist and how to build a client"; that one is "what's
// configured right now".
package llm

import "slices"

// Provider identifies an LLM vendor Lyra supports. The string values are the
// wire ids (Provider.id on the protocol, runs.start{provider}) and the catalog
// keys (models.dev) — lowercase, stable.
type Provider string

const (
	// Native + OpenAI-/Anthropic-compatible vendors with a catalog (models.list
	// browses their models). Each routes through its own adapter, which encodes
	// the vendor endpoint. IAM-only vendors (amazonbedrock, vertexai) are
	// intentionally absent — they don't fit the "paste an API key" model.
	ProviderAnthropic   Provider = "anthropic"
	ProviderOpenAI      Provider = "openai"
	ProviderMoonshot    Provider = "moonshot" // Kimi (OpenAI-compatible)
	ProviderDeepSeek    Provider = "deepseek" // DeepSeek (OpenAI-compatible)
	ProviderAlibaba     Provider = "alibaba"  // Qwen
	ProviderAzureOpenAI Provider = "azureopenai"
	ProviderFireworks   Provider = "fireworks"
	ProviderGoogle      Provider = "google" // Gemini
	ProviderGroq        Provider = "groq"
	ProviderHuggingface Provider = "huggingface"
	ProviderMinimax     Provider = "minimax"
	ProviderMistral     Provider = "mistral"
	ProviderOllama      Provider = "ollama" // local
	ProviderOpenRouter  Provider = "openrouter"
	ProviderPerplexity  Provider = "perplexity"
	ProviderTogether    Provider = "together"
	ProviderXAI         Provider = "xai" // Grok
	ProviderXiaomi      Provider = "xiaomi"
	ProviderZhipu       Provider = "zhipu" // GLM

	// Generic "bring-your-own-endpoint" providers: the user supplies the base
	// URL + key + model id, and the turn runs through the OpenAI- / Anthropic-
	// wire adapter. They cover any compatible gateway not named above (and have
	// no catalog — the model id is user-supplied).
	ProviderOpenAICompat    Provider = "openai-compatible"
	ProviderAnthropicCompat Provider = "anthropic-compatible"
)

// SupportedProviders lists every provider Lyra has an adapter for — the static
// set providers.list reports, regardless of which are configured. Sorted for a
// stable wire / CLI order.
func SupportedProviders() []Provider {
	out := make([]Provider, 0, len(providerInfo))
	for p := range providerInfo {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// IsSupported reports whether p is a known provider (has a table row).
func IsSupported(p Provider) bool {
	_, ok := providerInfo[p]
	return ok
}

// DefaultModel returns a provider's catalog default model id (used when the
// caller doesn't pick one). Empty for an unknown provider or one whose model id
// is always user-supplied (Azure deployment, Ollama, the generic passthroughs).
func DefaultModel(p Provider) string {
	return providerInfo[p].defaultModel
}

// APIKeyEnv returns the environment variable a provider's key is read from,
// or "" for an unknown provider.
func APIKeyEnv(p Provider) string {
	return providerInfo[p].apiKeyEnv
}

// RequiresBaseURL reports whether p has no built-in endpoint and needs a
// caller-supplied base URL (the generic passthroughs + Azure). The frontend
// renders a base URL field + free-form model input for these.
func RequiresBaseURL(p Provider) bool {
	return providerInfo[p].requiresBaseURL
}
