package llm

import (
	"context"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"

	"github.com/Tangerg/scope/models/alibaba"
	"github.com/Tangerg/scope/models/anthropic"
	"github.com/Tangerg/scope/models/azureopenai"
	"github.com/Tangerg/scope/models/deepseek"
	"github.com/Tangerg/scope/models/fireworks"
	"github.com/Tangerg/scope/models/google"
	"github.com/Tangerg/scope/models/groq"
	"github.com/Tangerg/scope/models/huggingface"
	"github.com/Tangerg/scope/models/minimax"
	"github.com/Tangerg/scope/models/mistral"
	"github.com/Tangerg/scope/models/moonshot"
	"github.com/Tangerg/scope/models/openai"
	"github.com/Tangerg/scope/models/openrouter"
	"github.com/Tangerg/scope/models/perplexity"
	"github.com/Tangerg/scope/models/together"
	"github.com/Tangerg/scope/models/xai"
	"github.com/Tangerg/scope/models/xiaomi"
	"github.com/Tangerg/scope/models/zhipu"
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

// buildFunc constructs the scope chat adapter for one (key, model, baseURL).
// One per provider — it's the only provider-specific code; everything else
// (validate / default-model / key-env) is data in [chatProviderCatalog].
type buildFunc func(spec ClientSpec, opts chat.Options) (chat.Model, error)

type chatProviderProfile struct {
	defaultModel string // catalog default model; "" when the model id is user-supplied
	apiKeyEnv    string
	build        buildFunc
	// requiresBaseURL marks providers with no built-in endpoint (the compatible
	// endpoint providers and Azure's per-resource URL): a base URL is mandatory,
	// validated at client build.
	requiresBaseURL bool
	// defaultBaseURL is a built-in endpoint used for live model discovery
	// when the caller configured none — set only for the local
	// Ollama daemon (hosted vendors encode their endpoint inside the adapter).
	defaultBaseURL string
}

// chatProviderCatalog is the data-driven provider table — the single place that knows
// each provider's adapter, default model, and key env var. A provider is
// "known" iff it has a row here; the supported / default-model / key-env /
// dispatch lookups all read this map. Most rows route through a vendor's
// OpenAI-compatible adapter (which encodes its own endpoint); the two generic
// compatible providers reuse the direct OpenAI / Anthropic adapters with a caller URL.
var chatProviderCatalog = map[Provider]chatProviderProfile{
	// Direct vendor wire adapters (base URL optional — defaults to the vendor endpoint).
	ProviderAnthropic: {defaultModel: defaultAnthropicModel, apiKeyEnv: "ANTHROPIC_API_KEY", build: buildAnthropicCountingModel},
	ProviderOpenAI:    {defaultModel: defaultOpenAIModel, apiKeyEnv: "OPENAI_API_KEY", build: buildOpenAIResponsesModel},
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
	ProviderOllama: {apiKeyEnv: "OLLAMA_API_KEY", defaultBaseURL: defaultOllamaOpenAIBaseURL, build: buildOllamaChatModel},

	// Azure: the base URL is the complete per-resource /openai/v1 endpoint;
	// the model id is a deployment name. Both are user-supplied.
	ProviderAzureOpenAI: {apiKeyEnv: "AZURE_OPENAI_API_KEY", requiresBaseURL: true, build: func(s ClientSpec, o chat.Options) (chat.Model, error) {
		return azureopenai.NewChat(azureopenai.ChatConfig{APIKey: s.APIKey, BaseURL: s.BaseURL, DefaultOptions: o})
	}},

	// Generic bring-your-own-endpoint providers: direct adapter + caller URL.
	ProviderOpenAICompatible:    {apiKeyEnv: "OPENAI_COMPATIBLE_API_KEY", requiresBaseURL: true, build: buildOpenAICompatibleModel},
	ProviderAnthropicCompatible: {apiKeyEnv: "ANTHROPIC_COMPATIBLE_API_KEY", requiresBaseURL: true, build: buildAnthropicCompatibleModel},
}

type anthropicCountingModel struct {
	*anthropic.Chat
}

var _ modelInputTokenCounter = (*anthropicCountingModel)(nil)

func (a *anthropicCountingModel) CountInputTokens(ctx context.Context, request *chat.Request) (int64, error) {
	return a.CountMessageInputTokens(ctx, request)
}

func buildAnthropicCountingModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	model, err := newAnthropicModel(spec, opts)
	if err != nil {
		return nil, err
	}
	return &anthropicCountingModel{Chat: model}, nil
}

func buildAnthropicCompatibleModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return newAnthropicModel(spec, opts)
}

func newAnthropicModel(spec ClientSpec, opts chat.Options) (*anthropic.Chat, error) {
	return anthropic.NewChat(anthropic.ChatConfig{
		APIKey:         spec.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.BaseURL,
	})
}

func buildOpenAIResponsesModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewResponsesChat(openai.ChatConfig{
		APIKey:         spec.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.BaseURL,
	})
}

func buildOpenAICompatibleModel(spec ClientSpec, opts chat.Options) (chat.Model, error) {
	return openai.NewChat(openai.ChatConfig{
		APIKey:         spec.APIKey,
		DefaultOptions: opts,
		BaseURL:        spec.BaseURL,
	})
}

// BuildClient wires a *chatclient.Client for one provider+model from [chatProviderCatalog]:
// it picks the model adapter, plugs in the model id, api key, and optional base
// URL. A provider that requires a base URL (a compatible endpoint provider or Azure)
// errors when one isn't supplied. Pricing is a separate accounting concern, so
// the constructed client carries no pricing hook.
func BuildClient(spec ClientSpec) (*chatclient.Client, error) {
	profile, ok := chatProviderCatalog[spec.Provider]
	if !ok {
		return nil, fmt.Errorf("llm: unsupported provider %q", spec.Provider)
	}
	if profile.requiresBaseURL && spec.BaseURL == "" {
		return nil, fmt.Errorf("llm: provider %q requires a base URL", spec.Provider)
	}

	opts := chat.Options{Model: spec.Model}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("llm: chat options for %q: %w", spec.Model, err)
	}

	model, err := profile.build(spec, opts)
	if err != nil {
		return nil, fmt.Errorf("llm: build %s model: %w", spec.Provider, err)
	}

	client, err := chatclient.New(classifyModelFailures(model), chatclient.Config{})
	if err != nil {
		return nil, fmt.Errorf("llm: chat client: %w", err)
	}
	return client, nil
}
