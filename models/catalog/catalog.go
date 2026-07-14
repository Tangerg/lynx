// Package catalog exposes the embedded model catalog: model identity, pricing,
// capabilities, modalities, and token limits. It is provider reference data,
// independent from Core model invocation protocols.
package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Provider is one provider's catalog entry.
type Provider struct {
	ID     string  `json:"id"`
	Models []Model `json:"models"`
}

type providerConfig struct {
	Provider string  `json:"provider"`
	Models   []Model `json:"models"`
}

//go:embed configs/*.json
var configs embed.FS

var entries = mustLoad()

func mustLoad() map[string]map[string]Model {
	files, err := fs.Glob(configs, "configs/*.json")
	if err != nil {
		panic(fmt.Errorf("catalog: glob configs: %w", err))
	}
	providers := make(map[string]map[string]Model, len(files))
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
		providers[strings.ToLower(config.Provider)] = models
	}
	return providers
}

// Lookup returns one model for a provider/model pair. Provider matching is
// case-insensitive. The returned value owns its slices.
func Lookup(provider, modelID string) (Model, bool) {
	models, ok := entries[strings.ToLower(provider)]
	if !ok {
		return Model{}, false
	}
	model, ok := models[modelID]
	if !ok {
		return Model{}, false
	}
	return cloneModel(model), true
}

// Models returns every cataloged model for provider, or nil when unknown.
// Order is unspecified and returned values own their slices.
func Models(provider string) []Model {
	models, ok := entries[strings.ToLower(provider)]
	if !ok {
		return nil
	}
	out := make([]Model, 0, len(models))
	for _, model := range models {
		out = append(out, cloneModel(model))
	}
	return out
}

// Get returns a provider's full catalog entry.
func Get(provider string) (Provider, bool) {
	models := Models(provider)
	if models == nil {
		return Provider{}, false
	}
	return Provider{ID: provider, Models: models}, true
}

func cloneModel(model Model) Model {
	model.Pricing = slices.Clone(model.Pricing)
	model.Reasoning.Levels = slices.Clone(model.Reasoning.Levels)
	model.Modalities.Input = slices.Clone(model.Modalities.Input)
	model.Modalities.Output = slices.Clone(model.Modalities.Output)
	return model
}
