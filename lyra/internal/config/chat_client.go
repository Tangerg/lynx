package config

import (
	"fmt"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/openai"
)

// BuildChatClient wires a *chat.Client from the loaded config — picks
// the right lynx model adapter, plugs in the model id and api key. It
// also returns the [engine.Pricing] cost hook derived from the model's
// metadata (nil when the model isn't in the pricing catalog), so turns
// can report CostUSD. Both are handed to engine.New via runtime.Config.
//
// M1 supports anthropic + openai. Adding a provider = one case in the
// switch + one import; the rest of Lyra doesn't care which model is
// behind the client.
func BuildChatClient(cfg Config) (*chat.Client, engine.Pricing, error) {
	opts, err := chat.NewOptions(cfg.Model)
	if err != nil {
		return nil, nil, fmt.Errorf("config: chat options for %q: %w", cfg.Model, err)
	}
	apiKey := model.NewAPIKey(cfg.APIKey)

	var llm chat.Model
	switch cfg.Provider {
	case ProviderAnthropic:
		llm, err = anthropic.NewChatModel(anthropic.ChatModelConfig{
			APIKey:         apiKey,
			DefaultOptions: opts,
		})
	case ProviderOpenAI:
		llm, err = openai.NewChatModel(openai.ChatModelConfig{
			APIKey:         apiKey,
			DefaultOptions: opts,
		})
	default:
		return nil, nil, fmt.Errorf("config: unsupported provider %q", cfg.Provider)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("config: build %s model: %w", cfg.Provider, err)
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
// than guessing. The served-model arg is ignored: lyra runs one
// configured model per client, so its rate card applies to every round.
func pricingFromMetadata(meta chat.ModelMetadata) engine.Pricing {
	if meta.Model.Pricing.IsZero() {
		return nil
	}
	rate := meta.Model.Pricing
	return func(_ string, u *chat.Usage) float64 {
		return rate.Cost(u)
	}
}
