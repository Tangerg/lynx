// Package pricing adapts catalog model pricing into kernel pricing hooks.
package pricing

import (
	"github.com/Tangerg/lynx/app/runtime/internal/kernel/accounting"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/models/catalog"
)

// Catalog returns the runtime's per-round cost hook: it prices the round's
// (provider, served model) from the models catalog, so a turn on any
// provider+model reports CostUSD.
func Catalog() accounting.Pricing {
	return func(provider, servedModel string, u *chat.Usage) float64 {
		if info, ok := catalog.Lookup(provider, servedModel); ok {
			return chat.CostOf(info.Pricing, u)
		}
		return 0
	}
}
