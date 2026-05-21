package config

import (
	"fmt"

	"github.com/Tangerg/lynx/core/model"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/lyra/internal/engine"
	"github.com/Tangerg/lynx/models/anthropic"
	"github.com/Tangerg/lynx/models/openai"
)

// EngineOnline maps the loaded config's online section into the
// engine-package OnlineConfig. Lives here so config knows about
// engine's shape, not the other way around — engine stays free of
// any config dependency.
func EngineOnline(cfg Config) engine.OnlineConfig {
	return engine.OnlineConfig{
		JinaAPIKey:       cfg.Online.JinaAPIKey,
		TavilyAPIKey:     cfg.Online.TavilyAPIKey,
		HTTPAllowedHosts: cfg.Online.HTTPAllowedHosts,
	}
}

// BuildChatClient wires a *chat.Client from the loaded config — picks
// the right lynx model adapter, plugs in the model id and api key.
// Returned client is the singleton handed to engine.New.
//
// M1 supports anthropic + openai. Adding a provider = one case in the
// switch + one import; the rest of Lyra doesn't care which model is
// behind the client.
func BuildChatClient(cfg Config) (*chat.Client, error) {
	opts, err := chat.NewOptions(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("config: chat options for %q: %w", cfg.Model, err)
	}
	apiKey := model.NewApiKey(cfg.APIKey)

	var llm chat.Model
	switch cfg.Provider {
	case ProviderAnthropic:
		llm, err = anthropic.NewChatModel(&anthropic.ChatModelConfig{
			ApiKey:         apiKey,
			DefaultOptions: opts,
		})
	case ProviderOpenAI:
		llm, err = openai.NewChatModel(&openai.ChatModelConfig{
			ApiKey:         apiKey,
			DefaultOptions: opts,
		})
	default:
		return nil, fmt.Errorf("config: unsupported provider %q", cfg.Provider)
	}
	if err != nil {
		return nil, fmt.Errorf("config: build %s model: %w", cfg.Provider, err)
	}

	client, err := chat.NewClient(llm)
	if err != nil {
		return nil, fmt.Errorf("config: chat client: %w", err)
	}
	return client, nil
}
