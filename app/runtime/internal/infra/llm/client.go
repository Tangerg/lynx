package llm

import (
	"fmt"

	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/models/alibaba"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/azureopenai"
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

const (
	defaultAnthropicModel = "claude-opus-5"
	defaultOpenAIModel    = "gpt-5.6-sol"
)

// ClientSpec is everything needed to build one chat client: which provider
// (the adapter to use), which model, the api key, and an optional endpoint
// override. It is the unit a multi-provider registry resolves a Run to.
type ClientSpec struct {
	Provider Provider
	Model    string
	APIKey   string
	BaseURL  string // empty uses the adapter's default endpoint
}

// buildFunc constructs the lynx chat adapter for one (key, model, baseURL).
// One per provider — it's the only provider-specific code; everything else
// (validate / default-model / key-env) is data in [providerInfo].
type buildFunc func(spec ClientSpec, opts chat.Options) (chat.Model, error)

type providerEntry struct {
	defaultModel string // catalog default model; "" when the model id is user-supplied
	apiKeyEnv    string
	build        buildFunc
	// requiresBaseURL marks providers with no built-in endpoint (the generic
	// compat passthroughs + Azure's per-resource URL): a base URL is mandatory,
	// validated at client build.
	requiresBaseURL bool
	// defaultBaseURL is a built-in endpoint used for live model discovery
	// when the caller configured none — set only for the local
	// Ollama daemon (hosted vendors encode their endpoint inside the adapter).
	defaultBaseURL string
}

// providerInfo is the data-driven provider table — the single place that knows
// each provider's adapter, default model, and key env var. A provider is
// "known" iff it has a row here; the supported / default-model / key-env /
// dispatch lookups all read this map. Most rows route through a vendor's
// OpenAI-compatible adapter (which encodes its own endpoint); the two generic
// passthroughs reuse the native OpenAI / Anthropic adapters with a caller URL.
var providerInfo = map[Provider]providerEntry{
	// Native wire adapters (base URL optional — defaults to the vendor endpoint).
	ProviderAnthropic: {defaultModel: defaultAnthropicModel, apiKeyEnv: "ANTHROPIC_API_KEY", build: anthropicNative},
	ProviderOpenAI:    {defaultModel: defaultOpenAIModel, apiKeyEnv: "OPENAI_API_KEY", build: openaiNative},
	ProviderGoogle: {defaultModel: google.ModelGemini36Flash, apiKeyEnv: "GOOGLE_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return google.NewChat(google.ChatConfig{APIKey: s.APIKey, DefaultOptions: o})
	}},

	// OpenAI-compatible vendors — each adapter encodes its own endpoint.
	ProviderMoonshot: {defaultModel: moonshot.ModelK3, apiKeyEnv: "MOONSHOT_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return moonshot.NewOpenAIChat(moonshot.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderDeepSeek: {defaultModel: deepseek.ModelV4Flash, apiKeyEnv: "DEEPSEEK_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return deepseek.NewOpenAIChat(deepseek.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderAlibaba: {defaultModel: alibaba.ModelQwen37Plus, apiKeyEnv: "ALIBABA_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return alibaba.NewOpenAIChat(alibaba.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderFireworks: {defaultModel: fireworks.ModelGPTOSS20B, apiKeyEnv: "FIREWORKS_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return fireworks.NewOpenAIChat(fireworks.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderGroq: {defaultModel: groq.ModelGPTOSS20B, apiKeyEnv: "GROQ_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return groq.NewOpenAIChat(groq.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderHuggingface: {defaultModel: huggingface.ModelGPTOSS120B, apiKeyEnv: "HUGGINGFACE_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return huggingface.NewOpenAIChat(huggingface.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderMinimax: {defaultModel: minimax.ModelM3, apiKeyEnv: "MINIMAX_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return minimax.NewOpenAIChat(minimax.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderMistral: {defaultModel: mistral.ModelSmall, apiKeyEnv: "MISTRAL_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return mistral.NewChat(mistral.ChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderOpenRouter: {defaultModel: openrouter.ModelAuto, apiKeyEnv: "OPENROUTER_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return openrouter.NewOpenAIChat(openrouter.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderPerplexity: {defaultModel: perplexity.ModelSonar, apiKeyEnv: "PERPLEXITY_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return perplexity.NewOpenAIChat(perplexity.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderTogether: {defaultModel: together.ModelRnj1Instruct, apiKeyEnv: "TOGETHER_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return together.NewOpenAIChat(together.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderXAI: {defaultModel: xai.ModelGrok45, apiKeyEnv: "XAI_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return xai.NewOpenAIChat(xai.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderXiaomi: {defaultModel: xiaomi.ModelV25Pro, apiKeyEnv: "XIAOMI_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return xiaomi.NewOpenAIChat(xiaomi.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},
	ProviderZhipu: {defaultModel: zhipu.ModelGLM52, apiKeyEnv: "ZHIPU_API_KEY", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return zhipu.NewOpenAIChat(zhipu.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},

	// Local daemon (base URL defaults to localhost; model id is user-pulled —
	// dynamic discovery probes the daemon's /v1/models for what is installed).
	ProviderOllama: {apiKeyEnv: "OLLAMA_API_KEY", defaultBaseURL: "http://localhost:11434/v1", build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return ollama.NewOpenAIChat(ollama.OpenAIChatConfig{APIKey: s.APIKey, DefaultOptions: o, BaseURL: s.BaseURL})
	}},

	// Azure: the base URL is the complete per-resource /openai/v1 endpoint;
	// the model id is a deployment name. Both are user-supplied.
	ProviderAzureOpenAI: {apiKeyEnv: "AZURE_OPENAI_API_KEY", requiresBaseURL: true, build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return azureopenai.NewChat(azureopenai.ChatConfig{APIKey: s.APIKey, BaseURL: s.BaseURL, DefaultOptions: o})
	}},

	// Generic bring-your-own-endpoint passthroughs: native adapter + caller URL.
	ProviderOpenAICompat:    {apiKeyEnv: "OPENAI_COMPATIBLE_API_KEY", requiresBaseURL: true, build: openaiNative},
	ProviderAnthropicCompat: {apiKeyEnv: "ANTHROPIC_COMPATIBLE_API_KEY", requiresBaseURL: true, build: anthropicNative},
}

// anthropicNative builds the native Anthropic adapter, threading an optional
// base URL (set for the anthropic-compatible passthrough).
func anthropicNative(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return anthropic.NewChat(anthropic.ChatConfig{
		APIKey:         spec.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.BaseURL,
	})
}

// openaiNative builds the native OpenAI adapter, threading an optional base URL
// (set for the openai-compatible passthrough).
func openaiNative(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewChat(openai.ChatConfig{
		APIKey:         spec.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.BaseURL,
	})
}

// BuildClient wires a *chatclient.Client for one provider+model from [providerInfo]:
// it picks the model adapter, plugs in the model id, api key, and optional base
// URL. A provider that requires a base URL (the generic passthroughs, Azure)
// errors when one isn't supplied. Pricing is a separate accounting concern, so
// the constructed client carries no pricing hook.
func BuildClient(spec ClientSpec) (*chatclient.Client, error) {
	entry, ok := providerInfo[spec.Provider]
	if !ok {
		return nil, fmt.Errorf("llm: unsupported provider %q", spec.Provider)
	}
	if entry.requiresBaseURL && spec.BaseURL == "" {
		return nil, fmt.Errorf("llm: provider %q requires a base URL", spec.Provider)
	}

	opts := chat.Options{Model: spec.Model}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("llm: chat options for %q: %w", spec.Model, err)
	}

	m, err := entry.build(spec, opts)
	if err != nil {
		return nil, fmt.Errorf("llm: build %s model: %w", spec.Provider, err)
	}

	client, err := chatclient.New(classifyModelFailures(m), chatclient.Config{})
	if err != nil {
		return nil, fmt.Errorf("llm: chat client: %w", err)
	}
	return client, nil
}
