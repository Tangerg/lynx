package runtime

import (
	"github.com/Tangerg/lynx/app/runtime/internal/infra/llm"
	"github.com/Tangerg/lynx/app/runtime/internal/kernel"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"
)

// CatalogPricing is the runtime's per-round cost hook: it prices the round's
// (provider, served model) from the models catalog, so a turn on any
// provider+model reports CostUSD.
func CatalogPricing() kernel.Pricing {
	return func(provider, servedModel string, u *chat.Usage) float64 {
		if provider != "" {
			if info, ok := catalog.Lookup(provider, servedModel); ok {
				return chat.CostOf(info.Pricing, u)
			}
			return 0
		}
		for _, p := range llm.SupportedProviders() {
			if info, ok := catalog.Lookup(string(p), servedModel); ok {
				return chat.CostOf(info.Pricing, u)
			}
		}
		return 0
	}
}
