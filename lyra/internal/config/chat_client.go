package config

import (
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/catalog"
	"github.com/Tangerg/lynx/models/deepseek"
	"github.com/Tangerg/lynx/models/moonshot"
	"github.com/Tangerg/lynx/models/openai"
	anthropicopt "github.com/anthropics/anthropic-sdk-go/option"
	openaiopt "github.com/openai/openai-go/v3/option"
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
// [BuildClient] — the runtime uses BuildClient directly to build other
// (provider, model) pairs once a provider registry lands.
func BuildChatClient(cfg Config) (*chat.Client, engine.Pricing, error) {
	return BuildClient(ClientSpec{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
}

// BuildClient wires a *chat.Client for one provider+model — picks the right
// lynx model adapter, plugs in the model id, api key, and optional base URL.
// It also returns the [engine.Pricing] cost hook derived from the model's
// metadata (nil when the model isn't in the pricing catalog), so turns can
// report CostUSD.
//
// Supports anthropic / openai / moonshot / deepseek (the last two via their
// OpenAI-compatible endpoints). Adding a provider = one case here + one
// import + a row in providerInfo; the rest of Lyra doesn't care which model
// is behind the client. Every case threads spec.BaseURL through — native
// openai/anthropic via a request option, the delegators via their BaseURL
// field.
func BuildClient(spec ClientSpec) (*chat.Client, engine.Pricing, error) {
	opts, err := chat.NewOptions(spec.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("config: chat options for %q: %w", spec.Model, err)
	}
	apiKey := model.NewAPIKey(spec.APIKey)

	var llm chat.Model
	switch spec.Provider {
	case ProviderAnthropic:
		var reqOpts []anthropicopt.RequestOption
		if spec.BaseURL != "" {
			reqOpts = append(reqOpts, anthropicopt.WithBaseURL(spec.BaseURL))
		}
		llm, err = anthropic.NewChatModel(anthropic.ChatModelConfig{
			APIKey:         apiKey,
			DefaultOptions: opts,
			RequestOptions: reqOpts,
		})
	case ProviderOpenAI:
		var reqOpts []openaiopt.RequestOption
		if spec.BaseURL != "" {
			reqOpts = append(reqOpts, openaiopt.WithBaseURL(spec.BaseURL))
		}
		llm, err = openai.NewChatModel(openai.ChatModelConfig{
			APIKey:         apiKey,
			DefaultOptions: opts,
			RequestOptions: reqOpts,
		})
	case ProviderMoonshot:
		// Kimi via Moonshot's OpenAI-compatible endpoint (domestic
		// api.moonshot.cn by default; BaseURL overrides for the intl region).
		llm, err = moonshot.NewOpenAIChatModel(moonshot.OpenAIChatModelConfig{
			APIKey:         apiKey,
			DefaultOptions: opts,
			BaseURL:        spec.BaseURL,
		})
	case ProviderDeepSeek:
		// DeepSeek via its OpenAI-compatible endpoint (api.deepseek.com by
		// default). Models: deepseek-v4-flash / -pro.
		llm, err = deepseek.NewOpenAIChatModel(deepseek.OpenAIChatModelConfig{
			APIKey:         apiKey,
			DefaultOptions: opts,
			BaseURL:        spec.BaseURL,
		})
	default:
		return nil, nil, fmt.Errorf("config: unsupported provider %q", spec.Provider)
	}
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
func pricingFromMetadata(meta chat.ModelMetadata) engine.Pricing {
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
// when the caller doesn't pick one). Empty for an unknown provider.
func DefaultModel(p Provider) string {
	return providerInfo[p].defaultModel
}

// CatalogPricing is the runtime's per-round cost hook once one runtime serves
// many models: it prices the SERVED model (the id the round reported) from
// the models catalog, scanning the supported providers for that model. This
// replaces the single-model pricingFromMetadata so a turn on any
// provider+model reports CostUSD, not just the default one. Unknown models
// price at zero (cost omitted rather than guessed).
func CatalogPricing() engine.Pricing {
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
