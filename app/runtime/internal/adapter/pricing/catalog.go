// Package pricing adapts catalog model pricing into kernel pricing hooks.
package pricing

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"
)

// Catalog returns the runtime's per-round cost hook: it prices the round's
// (provider, served model) from the models catalog, so a turn on any
// provider+model reports CostUSD.
func Catalog() accounting.Pricing {
	return func(provider, servedModel string, u *chat.Usage) float64 {
		if info, ok := catalog.Lookup(provider, servedModel); ok {
			usage := catalog.Usage{
				InputTokens:  u.PromptTokens,
				OutputTokens: u.CompletionTokens,
			}
			if u.CacheReadInputTokens != nil {
				usage.CacheReadInputTokens = *u.CacheReadInputTokens
			}
			if u.CacheWriteInputTokens != nil {
				usage.CacheWriteInputTokens = *u.CacheWriteInputTokens
			}
			return catalog.CostOf(info.Pricing, usage)
		}
		return 0
	}
}
