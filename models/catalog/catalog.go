// Package catalog is a small embedded catalog of per-model metadata —
// pricing, capabilities, and identity — modeled on charm.land/catwalk:
// the data lives in per-provider JSON configs under configs/, is embedded
// via go:embed, and is exposed through [Lookup] and [Models]. Provider
// adapters use it to populate chat.ModelMetadata.Model so consumers can
// attribute USD cost and read capabilities without hard-coding tables.
//
// Maintenance: edit (or regenerate) the configs/<provider>.json files —
// each lists its models with rates and capabilities. Only the providers
// lynx ships adapters for are covered; add a config + nothing else to
// extend (the embed is a glob).
package catalog

import (
	"embed"
	"encoding/json"
	"io/fs"
	"strings"

	"github.com/Tangerg/lynx/core/model/chat"
)

// configs holds one JSON file per provider. Adding a provider is just
// dropping a configs/<provider>.json — no code change (glob embed).
//
//go:embed configs/*.json
var configs embed.FS

type providerConfig struct {
	Provider string           `json:"provider"`
	Models   []chat.ModelInfo `json:"models"`
}

// catalog maps provider -> model id -> info, built once from every
// embedded config.
var catalog = mustLoad()

func mustLoad() map[string]map[string]chat.ModelInfo {
	files, err := fs.Glob(configs, "configs/*.json")
	if err != nil {
		panic("catalog: glob configs: " + err.Error())
	}
	out := make(map[string]map[string]chat.ModelInfo, len(files))
	for _, name := range files {
		raw, err := configs.ReadFile(name)
		if err != nil {
			panic("catalog: read " + name + ": " + err.Error())
		}
		var cfg providerConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			// Embedded, compile-time data — a parse failure is a build
			// error in our own configs, so fail fast (mirrors
			// regexp.MustCompile). Tests cover the configs parsing.
			panic("catalog: invalid config " + name + ": " + err.Error())
		}
		byModel := make(map[string]chat.ModelInfo, len(cfg.Models))
		for _, m := range cfg.Models {
			byModel[m.ID] = m
		}
		out[strings.ToLower(cfg.Provider)] = byModel
	}
	return out
}

// Lookup returns the model info for (provider, modelID). The provider is
// matched case-insensitively (adapter Provider consts are capitalized,
// e.g. "Anthropic", while configs use lowercase ids). ok is false when
// the pair isn't cataloged — the caller treats that as "metadata
// unknown" (a zero chat.ModelInfo).
func Lookup(provider, modelID string) (chat.ModelInfo, bool) {
	if byModel, ok := catalog[strings.ToLower(provider)]; ok {
		if m, ok := byModel[modelID]; ok {
			return m, true
		}
	}
	return chat.ModelInfo{}, false
}

// Models returns every cataloged model for a provider (case-insensitive),
// or nil when the provider isn't cataloged. Order is unspecified.
func Models(provider string) []chat.ModelInfo {
	byModel, ok := catalog[strings.ToLower(provider)]
	if !ok {
		return nil
	}
	out := make([]chat.ModelInfo, 0, len(byModel))
	for _, m := range byModel {
		out = append(out, m)
	}
	return out
}
