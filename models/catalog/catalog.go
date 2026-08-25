// Package catalog exposes the embedded model catalog: model identity, pricing,
// capabilities, modalities, and token limits. It is provider reference data,
// independent from Core model invocation protocols.
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"
)

// Provider is one provider's catalog entry.
type Provider struct {
	ID     string  `json:"id"`
	Models []Model `json:"models"`
}

// Clone returns a caller-owned copy of the provider and all nested models.
func (p Provider) Clone() Provider {
	p.Models = make([]Model, len(p.Models))
	for index := range p.Models {
		p.Models[index] = p.Models[index].Clone()
	}
	return p
}

// Catalog owns access to the repository's embedded provider and model data.
// Its zero value is ready to use and carries no mutable per-caller state.
type Catalog struct{}

// Default is the repository's embedded model catalog.
var Default Catalog

type providerConfig struct {
	Provider string  `json:"provider"`
	Models   []Model `json:"models"`
}

//go:embed configs/*.json
var configs embed.FS

type catalogEntry struct {
	provider Provider
	models   map[string]Model
}

var entries = mustLoad()

func mustLoad() map[string]catalogEntry {
	files, err := fs.Glob(configs, "configs/*.json")
	if err != nil {
		panic(fmt.Errorf("catalog: glob configs: %w", err))
	}
	providers := make(map[string]catalogEntry, len(files))
	for _, name := range files {
		raw, err := configs.ReadFile(name)
		if err != nil {
			panic(fmt.Errorf("catalog: read %s: %w", name, err))
		}
		var config providerConfig
		if err := json.Unmarshal(raw, &config); err != nil {
			panic(fmt.Errorf("catalog: invalid config %s: %w", name, err))
		}
		models := make(map[string]Model, len(config.Models))
		for _, model := range config.Models {
			models[model.ID] = model
		}
		providers[strings.ToLower(config.Provider)] = catalogEntry{
			provider: Provider{ID: config.Provider, Models: config.Models},
			models:   models,
		}
	}
	return providers
}

// Lookup returns one model for a provider/model pair. Provider matching is
// case-insensitive. The returned value owns its slices.
func (Catalog) Lookup(provider, modelID string) (Model, bool) {
	entry, ok := entries[strings.ToLower(provider)]
	if !ok {
		return Model{}, false
	}
	model, ok := entry.models[modelID]
	if !ok {
		return Model{}, false
	}
	return model.Clone(), true
}

// Models returns every cataloged model for provider, or nil when unknown.
// Order is unspecified and returned values own their slices.
func (Catalog) Models(provider string) []Model {
	entry, ok := entries[strings.ToLower(provider)]
	if !ok {
		return nil
	}
	return entry.provider.Clone().Models
}

// Provider returns one provider's complete catalog entry. Provider matching is
// case-insensitive, while the returned ID preserves the catalog's canonical
// spelling.
func (Catalog) Provider(provider string) (Provider, bool) {
	entry, ok := entries[strings.ToLower(provider)]
	if !ok {
		return Provider{}, false
	}
	return entry.provider.Clone(), true
}
