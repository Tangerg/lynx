package config

import (
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
)

// BuildChatClient wires the single configured provider into a *chat.Client
// from the loaded config. Thin wrapper over [llm.BuildClient] — the runtime's
// clientResolver calls llm.BuildClient directly to build other (provider,
// model) pairs from the provider registry.
func BuildChatClient(cfg Config) (*chat.Client, error) {
	return llm.BuildClient(llm.ClientSpec{
		Provider: cfg.Provider,
		Model:    cfg.Model,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
	})
}
