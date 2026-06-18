package config

import (
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/models/catalog"
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

// CatalogPricing is the runtime's per-round cost hook: it prices the SERVED
// model (the id the round reported) from the models catalog, scanning the
// supported providers for that model, so a turn on any provider+model reports
// CostUSD. Unknown models price at zero (cost omitted rather than guessed).
// It's wired here (composition) rather than in [llm] because it depends on the
// kernel cost-hook type, which infra must not import.
func CatalogPricing() kernel.Pricing {
	providers := llm.SupportedProviders()
	return func(servedModel string, u *chat.Usage) float64 {
		for _, p := range providers {
			if info, ok := catalog.Lookup(string(p), servedModel); ok {
				return chat.CostOf(info.Pricing, u)
			}
		}
		return 0
	}
}
