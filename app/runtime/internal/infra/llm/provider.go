// Package llm owns Lyra's static provider catalog and constructs a chat client
// for a selected provider. Each catalog entry binds a vendor, default model,
// credential environment key, and external wire adapter.
//
// The runtime-mutable credential registry (a provider's configured key + base
// URL) is a separate concern. This package answers what providers exist and how
// to construct their clients, not what credentials are configured now.
package llm

import (
	"os"
	"slices"
)

// Provider identifies an LLM vendor Lyra supports. Its lowercase string value
// is the stable catalog key.
type Provider string

const (
	// Named vendors with a model catalog. Each routes through its own adapter,
	// which encodes the vendor endpoint. IAM-only vendors (amazonbedrock, vertexai) are
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
	// URL + key + model id, and the Run executes through the OpenAI- / Anthropic-
	// wire adapter. They cover any compatible gateway not named above (and have
	// no catalog — the model id is user-supplied).
	ProviderOpenAICompatible    Provider = "openai-compatible"
	ProviderAnthropicCompatible Provider = "anthropic-compatible"
)

// SupportedProviders lists every provider with a static catalog entry,
// regardless of which are configured. The result has deterministic order.
func SupportedProviders() []Provider {
	out := make([]Provider, 0, len(chatProviderCatalog))
	for provider := range chatProviderCatalog {
		out = append(out, provider)
	}
	slices.Sort(out)
	return out
}

// IsSupported reports whether p is a known provider (has a table row).
func (p Provider) IsSupported() bool {
	_, ok := chatProviderCatalog[p]
	return ok
}

// DefaultModel returns a provider's catalog default model id (used when the
// caller doesn't pick one). Empty for an unknown provider or one whose model id
// is always user-supplied (Azure deployment, Ollama, or a compatible endpoint).
func (p Provider) DefaultModel() string {
	return chatProviderCatalog[p].defaultModel
}

// APIKeyEnv returns the environment variable a provider's key is read from,
// or "" for an unknown provider.
func (p Provider) APIKeyEnv() string {
	return chatProviderCatalog[p].apiKeyEnv
}

// RequiresBaseURL reports whether p has no built-in endpoint and needs a
// caller-supplied base URL (the compatible endpoint providers and Azure).
func (p Provider) RequiresBaseURL() bool {
	return chatProviderCatalog[p].requiresBaseURL
}

// DefaultBaseURL returns a provider's built-in endpoint used for live model
// discovery when the caller configured none — non-empty only for the local
// Ollama daemon (hosted vendors encode their endpoint inside the adapter, and
// the compatible endpoint providers have no default at all).
func (p Provider) DefaultBaseURL() string {
	return chatProviderCatalog[p].defaultBaseURL
}

// ProbeModels reports whether p's available models are defined by its live
// endpoint rather than the static catalog — true exactly for the providers
// whose model id is user-supplied (no catalog default): Ollama, Azure, and the
// generic OpenAI-/Anthropic-compatible endpoints. Dynamic discovery probes their
// /v1/models instead of serving the embedded catalog for these.
func (p Provider) ProbeModels() bool {
	profile, ok := chatProviderCatalog[p]
	return ok && profile.defaultModel == ""
}

// EnvKeys reads the environment once and returns the API keys present for the
// providers a key alone makes usable — keyed by provider id, value the key. It
// backs the provider registry's stored>env credential fallback (a developer
// with ANTHROPIC_API_KEY / OPENAI_API_KEY / … in their shell gets those
// providers enabled out of the box).
//
// Providers that require a caller-supplied base URL (Azure and the compatible
// endpoint providers) are excluded: an env key alone can't reach their
// endpoint, so surfacing them as "enabled from env" would be a lie. The
// environment is process-static, so callers read this once at startup.
func EnvKeys() map[string]string {
	out := make(map[string]string)
	for provider, profile := range chatProviderCatalog {
		if profile.requiresBaseURL || profile.apiKeyEnv == "" {
			continue
		}
		if key := os.Getenv(profile.apiKeyEnv); key != "" {
			out[string(provider)] = key
		}
	}
	return out
}
