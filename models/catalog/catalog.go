// Package catalog is the public, read-only view of the embedded
// per-model metadata catalog — which models a provider offers and each
// model's identity, pricing, capabilities, and limits.
//
// It's a thin facade over the module-internal catalog (which the provider
// adapters use to populate chat.ModelMetadata). The data is sourced from
// models.dev; this package exists so downstream modules can enumerate a
// provider's models for a UI (e.g. a "models.list" surface) without
// constructing a client per model — chat.Model.Metadata only ever carries
// one model's info, so listing needs the catalog directly.
//
// Lookups are case-insensitive on the provider name (adapter Provider
// consts are capitalized, e.g. "Anthropic"; the configs use lowercase ids).
package catalog

import (
	"github.com/Tangerg/lynx/core/model/chat"
	internalcatalog "github.com/Tangerg/lynx/models/internal/catalog"
)

// Provider is one provider's catalog entry: its id plus every model the
// catalog knows it offers. It's the public mirror of the internal
// per-provider config.
type Provider struct {
	ID     string           `json:"id"`
	Models []chat.ModelInfo `json:"models"`
}

// Models returns every cataloged model for a provider (case-insensitive),
// or nil when the provider isn't cataloged. Order is unspecified.
func Models(provider string) []chat.ModelInfo {
	return internalcatalog.Models(provider)
}

// Lookup returns the metadata for one (provider, modelID) pair. ok is false
// when the pair isn't cataloged — callers treat that as "metadata unknown".
func Lookup(provider, modelID string) (chat.ModelInfo, bool) {
	return internalcatalog.Lookup(provider, modelID)
}

// Get returns a provider's full catalog entry. ok is false when the
// provider has no catalog (no models known).
func Get(provider string) (Provider, bool) {
	models := internalcatalog.Models(provider)
	if models == nil {
		return Provider{}, false
	}
	return Provider{ID: provider, Models: models}, true
}
