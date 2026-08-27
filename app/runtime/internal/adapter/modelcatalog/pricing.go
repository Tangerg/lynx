package modelcatalog

import (
	"github.com/Tangerg/scope/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

// Pricing returns a per-round cost calculator backed by the model
// catalog. It prices the round's provider and served model, so every Run
// reports CostUSD against the model that actually answered.
func Pricing() accounting.Pricing {
	return func(provider, servedModel string, usage *chat.Usage) float64 {
		if info, ok := catalog.Default.Lookup(provider, servedModel); ok {
			catalogUsage := catalog.Usage{
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			}
			if usage.CacheReadInputTokens != nil {
				catalogUsage.CacheReadInputTokens = *usage.CacheReadInputTokens
			}
			if usage.CacheWriteInputTokens != nil {
				catalogUsage.CacheWriteInputTokens = *usage.CacheWriteInputTokens
			}
			return info.Pricing.Cost(catalogUsage)
		}
		return 0
	}
}
