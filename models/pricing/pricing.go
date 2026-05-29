// Package pricing is a small embedded catalog of per-model token rates,
// modeled on charm.land/catwalk: the data lives in per-provider JSON
// configs under configs/, is embedded via go:embed, and is exposed
// through a single [Lookup]. Provider adapters use it to populate
// model.ModelMetadata.Pricing so consumers can attribute USD cost
// without hard-coding rates.
//
// Maintenance: edit (or regenerate) the configs/<provider>.json files —
// each lists its models with input/output/cache per-1M-token rates. Only
// the providers lynx ships adapters for are covered; add a config + an
// embed line to extend.
package pricing

import (
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/Tangerg/lynx/core/model"
)

//go:embed configs/anthropic.json
var anthropicConfig []byte

//go:embed configs/openai.json
var openaiConfig []byte

// entry is one model's row: its id plus the embedded rate card (the
// model.Pricing json tags flatten into the same JSON object).
type entry struct {
	ID string `json:"id"`
	model.Pricing
}

type providerConfig struct {
	Provider string  `json:"provider"`
	Models   []entry `json:"models"`
}

// catalog maps provider -> model id -> rate card, built once from the
// embedded configs.
var catalog = mustLoad(anthropicConfig, openaiConfig)

func mustLoad(configs ...[]byte) map[string]map[string]model.Pricing {
	out := make(map[string]map[string]model.Pricing, len(configs))
	for _, raw := range configs {
		var cfg providerConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			// Embedded, compile-time data — a parse failure is a build
			// error in our own configs, so fail fast (mirrors
			// regexp.MustCompile). Tests cover the configs parsing.
			panic("pricing: invalid embedded config: " + err.Error())
		}
		byModel := make(map[string]model.Pricing, len(cfg.Models))
		for _, e := range cfg.Models {
			byModel[e.ID] = e.Pricing
		}
		out[strings.ToLower(cfg.Provider)] = byModel
	}
	return out
}

// Lookup returns the rate card for (provider, modelID). The provider is
// matched case-insensitively (adapter Provider consts are capitalized,
// e.g. "Anthropic", while configs use lowercase ids). ok is false when
// the pair isn't cataloged — the caller treats that as "pricing
// unknown" (a zero model.Pricing).
func Lookup(provider, modelID string) (model.Pricing, bool) {
	if byModel, ok := catalog[strings.ToLower(provider)]; ok {
		if p, ok := byModel[modelID]; ok {
			return p, true
		}
	}
	return model.Pricing{}, false
}
