// Package catalog is an embedded catalog of per-model metadata — pricing,
// capabilities (modalities, reasoning, tool calling, structured output),
// limits, and identity. The data lives in per-provider JSON configs under
// configs/, is embedded via go:embed, and is exposed through [Lookup] and
// [Models]. Provider adapters use it to populate chat.ModelMetadata.Model
// so consumers can attribute USD cost and read capabilities without
// hard-coding tables.
//
// The configs are generated from models.dev (a community model database,
// also used by LangChain's model profiles), with reasoning effort levels
// backfilled from charm.land/catwalk. It's internal to the models module:
// adapters fill metadata, and downstream code reads it via a Model's
// Metadata, not by importing this package.
//
// Maintenance: regenerate the configs/<provider>.json files from
// models.dev. Only the providers lynx ships adapters for are covered; add
// a config + nothing else to extend (the embed is a glob).
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
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
		panic(fmt.Errorf("catalog: glob configs: %w", err))
	}
	out := make(map[string]map[string]chat.ModelInfo, len(files))
	for _, name := range files {
		raw, err := configs.ReadFile(name)
		if err != nil {
			panic(fmt.Errorf("catalog: read %s: %w", name, err))
		}
		var cfg providerConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			// Embedded, compile-time data — a parse failure is a build
			// error in our own configs, so fail fast (mirrors
			// regexp.MustCompile). Tests cover the configs parsing.
			panic(fmt.Errorf("catalog: invalid config %s: %w", name, err))
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

// Resolve builds a [chat.ModelMetadata] for a provider, filling Model from
// the catalog by the default model id. It's the one place adapters share:
// the caller's override (a non-nil Config.Metadata) wins as-is, otherwise
// the lookup keys off the override's (or provider's) name — so OpenAI- and
// Anthropic-compat delegators (deepseek, vertexai, …) that pass their own
// Provider resolve against their own config. opts may be nil.
func Resolve(provider string, opts *chat.Options, override *chat.ModelMetadata) chat.ModelMetadata {
	md := chat.ModelMetadata{Provider: provider}
	if override != nil {
		md = *override
	}
	if md.Model.IsZero() && opts != nil {
		if m, ok := Lookup(md.Provider, opts.Model); ok {
			md.Model = m
		}
	}
	return md
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
