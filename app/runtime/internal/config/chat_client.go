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

// CatalogPricing is the runtime's per-round cost hook: it prices the round's
// (provider, served model) from the models catalog, so a turn on any
// provider+model reports CostUSD. Pricing keys on the PROVIDER, not just the
// model id — the same id (e.g. gpt-4o) is priced differently across providers
// (openai vs azureopenai), so a model-id-only lookup would mis-attribute the
// rate. A provider whose catalog lacks the served model (a custom / compat
// model, or a provider-routed fallback) prices at zero — no price beats a wrong
// price from another provider's catalog. It's wired here (composition) rather
// than in [llm] because it depends on the kernel cost-hook type, which infra
// must not import.
func CatalogPricing() kernel.Pricing {
	return func(provider, servedModel string, u *chat.Usage) float64 {
		if provider != "" {
			if info, ok := catalog.Lookup(provider, servedModel); ok {
				return chat.CostOf(info.Pricing, u)
			}
			return 0
		}
		// No provider supplied (no per-run selection and no configured default) —
		// last-resort scan so an otherwise-unattributable round still prices.
		for _, p := range llm.SupportedProviders() {
			if info, ok := catalog.Lookup(string(p), servedModel); ok {
				return chat.CostOf(info.Pricing, u)
			}
		}
		return 0
	}
}
