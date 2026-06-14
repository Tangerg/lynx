package config

import (
	"fmt"
	"slices"

	anthropicopt "github.com/anthropics/anthropic-sdk-go/option"
	openaiopt "github.com/openai/openai-go/v3/option"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/kernel"

	"github.com/Tangerg/lynx/models/alibaba"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/azureopenai"
	"github.com/Tangerg/lynx/models/catalog"
	"github.com/Tangerg/lynx/models/deepseek"
	"github.com/Tangerg/lynx/models/fireworks"
	"github.com/Tangerg/lynx/models/google"
	"github.com/Tangerg/lynx/models/groq"
	"github.com/Tangerg/lynx/models/huggingface"
	"github.com/Tangerg/lynx/models/minimax"
	"github.com/Tangerg/lynx/models/mistral"
	"github.com/Tangerg/lynx/models/moonshot"
	"github.com/Tangerg/lynx/models/ollama"
	"github.com/Tangerg/lynx/models/openai"
	"github.com/Tangerg/lynx/models/openrouter"
	"github.com/Tangerg/lynx/models/perplexity"
	"github.com/Tangerg/lynx/models/together"
	"github.com/Tangerg/lynx/models/xai"
	"github.com/Tangerg/lynx/models/xiaomi"
	"github.com/Tangerg/lynx/models/zhipu"
)

// ClientSpec is everything needed to build one chat client: which provider
// (the adapter to use), which model, the api key, and an optional endpoint
// override. It's the unit a multi-provider registry resolves a turn to —
// [BuildChatClient] builds the single configured one, and the runtime
// builds one per (provider, model) on demand.
type ClientSpec struct {
	Provider Provider
	Model    string
	APIKey   string
	BaseURL  string // empty uses the adapter's default endpoint
}

// BuildChatClient wires the single configured provider into a *chat.Client
// (plus its cost hook), from the loaded config. Thin wrapper over
// [BuildClient] — the runtime's clientResolver uses BuildClient directly
// to build other (provider, model) pairs from the provider registry.
func BuildChatClient(cfg Config) (*chat.Client, kernel.Pricing, error) {
	return BuildClient(ClientSpec{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
}

// buildFunc constructs the lynx chat adapter for one (key, model, baseURL).
// One per provider — it's the only provider-specific code; everything else
// (validate / default-model / key-env / pricing) is data in [providerInfo].
type buildFunc func(spec ClientSpec, opts *chat.Options) (chat.Model, error)

type providerEntry struct {
	defaultModel string // catalog default model; "" when the model id is user-supplied
	apiKeyEnv    string
	build        buildFunc
	// requiresBaseURL marks providers with no built-in endpoint (the generic
	// compat passthroughs + Azure's per-resource URL): a base URL is mandatory,
	// validated at client build.
	requiresBaseURL bool
}

// providerInfo is the data-driven provider table — the single place that knows
// each provider's adapter, default model, and key env var. A provider is
// "known" iff it has a row here; the validate / default-model / key-env /
// dispatch lookups all read this map. Most rows route through a vendor's
// OpenAI-compatible adapter (which encodes its own endpoint); the two generic
// passthroughs reuse the native OpenAI / Anthropic adapters with a caller URL.
//
// NOTE: this lives in config for now; the planned follow-up extracts a
// dedicated provider-management module (registry + catalog + construction).
var providerInfo = map[Provider]providerEntry{
	// Native wire adapters (base URL optional — defaults to the vendor endpoint).
	ProviderAnthropic: {defaultModel: "claude-3-5-haiku-20241022", apiKeyEnv: "ANTHROPIC_API_KEY", build: anthropicNative},
	ProviderOpenAI:    {defaultModel: "gpt-4o-mini", apiKeyEnv: "OPENAI_API_KEY", build: openaiNative},
	ProviderGoogle: {defaultModel: "gemini-2.0-flash-lite", apiKeyEnv: "GOOGLE_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return google.NewChatModel(google.ChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o})
	}},

	// OpenAI-compatible vendors — each adapter encodes its own endpoint.
	ProviderMoonshot: {defaultModel: "kimi-k2-0905-preview", apiKeyEnv: "MOONSHOT_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return moonshot.NewOpenAIChatModel(moonshot.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderDeepSeek: {defaultModel: "deepseek-v4-flash", apiKeyEnv: "DEEPSEEK_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return deepseek.NewOpenAIChatModel(deepseek.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderAlibaba: {defaultModel: "qwen-flash", apiKeyEnv: "ALIBABA_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return alibaba.NewOpenAIChatModel(alibaba.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderFireworks: {defaultModel: "accounts/fireworks/models/gpt-oss-20b", apiKeyEnv: "FIREWORKS_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return fireworks.NewOpenAIChatModel(fireworks.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderGroq: {defaultModel: "llama-3.1-8b-instant", apiKeyEnv: "GROQ_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return groq.NewOpenAIChatModel(groq.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderHuggingface: {defaultModel: "XiaomiMiMo/MiMo-V2-Flash", apiKeyEnv: "HUGGINGFACE_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return huggingface.NewOpenAIChatModel(huggingface.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderMinimax: {defaultModel: "MiniMax-M2", apiKeyEnv: "MINIMAX_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return minimax.NewOpenAIChatModel(minimax.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderMistral: {defaultModel: "ministral-3b-latest", apiKeyEnv: "MISTRAL_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return mistral.NewOpenAIChatModel(mistral.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderOpenRouter: {defaultModel: "inclusionai/ling-2.6-flash", apiKeyEnv: "OPENROUTER_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return openrouter.NewOpenAIChatModel(openrouter.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderPerplexity: {defaultModel: "sonar", apiKeyEnv: "PERPLEXITY_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return perplexity.NewOpenAIChatModel(perplexity.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderTogether: {defaultModel: "essentialai/Rnj-1-Instruct", apiKeyEnv: "TOGETHER_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return together.NewOpenAIChatModel(together.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderXAI: {defaultModel: "grok-build-0.1", apiKeyEnv: "XAI_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return xai.NewOpenAIChatModel(xai.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderXiaomi: {defaultModel: "mimo-v2-flash", apiKeyEnv: "XIAOMI_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return xiaomi.NewOpenAIChatModel(xiaomi.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderZhipu: {defaultModel: "glm-4.7-flashx", apiKeyEnv: "ZHIPU_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return zhipu.NewOpenAIChatModel(zhipu.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},

	// Local daemon (base URL defaults to localhost; model id is user-pulled).
	ProviderOllama: {apiKeyEnv: "OLLAMA_API_KEY", build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return ollama.NewOpenAIChatModel(ollama.OpenAIChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), DefaultOptions: o, BaseURL: s.BaseURL})
	}},

	// Azure: the base URL is the per-resource endpoint; the model id is a
	// deployment name. Both are user-supplied, so requiresBaseURL.
	ProviderAzureOpenAI: {apiKeyEnv: "AZURE_OPENAI_API_KEY", requiresBaseURL: true, build: func(s ClientSpec, o *chat.Options) (chat.Model, error) {
		return azureopenai.NewChatModel(azureopenai.ChatModelConfig{APIKey: model.NewAPIKey(s.APIKey), Endpoint: s.BaseURL, DefaultOptions: o})
	}},

	// Generic bring-your-own-endpoint passthroughs: native adapter + caller URL.
	ProviderOpenAICompat:    {apiKeyEnv: "OPENAI_COMPATIBLE_API_KEY", requiresBaseURL: true, build: openaiNative},
	ProviderAnthropicCompat: {apiKeyEnv: "ANTHROPIC_COMPATIBLE_API_KEY", requiresBaseURL: true, build: anthropicNative},
}

// anthropicNative builds the native Anthropic adapter, threading an optional
// base URL (set for the anthropic-compatible passthrough).
func anthropicNative(spec ClientSpec, opts *chat.Options) (chat.Model, error) {
	var reqOpts []anthropicopt.RequestOption
	if spec.BaseURL != "" {
		reqOpts = append(reqOpts, anthropicopt.WithBaseURL(spec.BaseURL))
	}
	return anthropic.NewChatModel(anthropic.ChatModelConfig{
		APIKey:         model.NewAPIKey(spec.APIKey),
		DefaultOptions: opts,
		RequestOptions: reqOpts,
	})
}

// openaiNative builds the native OpenAI adapter, threading an optional base URL
// (set for the openai-compatible passthrough).
func openaiNative(spec ClientSpec, opts *chat.Options) (chat.Model, error) {
	var reqOpts []openaiopt.RequestOption
	if spec.BaseURL != "" {
		reqOpts = append(reqOpts, openaiopt.WithBaseURL(spec.BaseURL))
	}
	return openai.NewChatModel(openai.ChatModelConfig{
		APIKey:         model.NewAPIKey(spec.APIKey),
		DefaultOptions: opts,
		RequestOptions: reqOpts,
	})
}

// BuildClient wires a *chat.Client for one provider+model from [providerInfo]:
// it picks the model adapter, plugs in the model id, api key, and optional base
// URL, and returns the [kernel.Pricing] cost hook derived from the model's
// metadata (nil when the model isn't in the pricing catalog) so turns can
// report CostUSD. A provider that requires a base URL (the generic passthroughs,
// Azure) errors when one isn't supplied.
func BuildClient(spec ClientSpec) (*chat.Client, kernel.Pricing, error) {
	entry, ok := providerInfo[spec.Provider]
	if !ok {
		return nil, nil, fmt.Errorf("config: unsupported provider %q", spec.Provider)
	}
	if entry.requiresBaseURL && spec.BaseURL == "" {
		return nil, nil, fmt.Errorf("config: provider %q requires a base URL", spec.Provider)
	}

	opts, err := chat.NewOptions(spec.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("config: chat options for %q: %w", spec.Model, err)
	}

	llm, err := entry.build(spec, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("config: build %s model: %w", spec.Provider, err)
	}

	client, err := chat.NewClient(llm)
	if err != nil {
		return nil, nil, fmt.Errorf("config: chat client: %w", err)
	}
	return client, pricingFromMetadata(llm.Metadata()), nil
}

// pricingFromMetadata turns a model's rate card (set by the adapter from
// the model catalog) into the engine's per-round cost hook. Returns nil
// when pricing is unknown — the engine then leaves CostUSD at zero rather
// than guessing. The served-model arg is ignored: one client carries one
// model, so its rate card applies to every round.
func pricingFromMetadata(meta chat.ModelMetadata) kernel.Pricing {
	if len(meta.Model.Pricing) == 0 {
		return nil
	}
	bands := meta.Model.Pricing
	return func(_ string, u *chat.Usage) float64 {
		return chat.CostOf(bands, u)
	}
}

// SupportedProviders lists the providers Lyra has an adapter for — the
// static set providers.list reports, regardless of which are configured.
// Sorted for a stable wire / CLI order.
func SupportedProviders() []Provider {
	out := make([]Provider, 0, len(providerInfo))
	for p := range providerInfo {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

// DefaultModel returns a provider's catalog default model id (the one used
// when the caller doesn't pick one). Empty for an unknown provider or one
// whose model id is always user-supplied (Azure deployment, Ollama, the
// generic passthroughs).
func DefaultModel(p Provider) string {
	return providerInfo[p].defaultModel
}

// RequiresBaseURL reports whether p has no built-in endpoint and needs a
// caller-supplied base URL (the generic passthroughs + Azure). The frontend
// renders a base URL field + free-form model input for these.
func RequiresBaseURL(p Provider) bool {
	return providerInfo[p].requiresBaseURL
}

// CatalogPricing is the runtime's per-round cost hook once one runtime serves
// many models: it prices the SERVED model (the id the round reported) from
// the models catalog, scanning the supported providers for that model. This
// replaces the single-model pricingFromMetadata so a turn on any
// provider+model reports CostUSD, not just the default one. Unknown models
// price at zero (cost omitted rather than guessed).
func CatalogPricing() kernel.Pricing {
	providers := SupportedProviders()
	return func(servedModel string, u *chat.Usage) float64 {
		for _, p := range providers {
			if info, ok := catalog.Lookup(string(p), servedModel); ok {
				return chat.CostOf(info.Pricing, u)
			}
		}
		return 0
	}
}
