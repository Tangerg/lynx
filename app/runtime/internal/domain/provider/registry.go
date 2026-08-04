// Package provider models the credentials and enablement registry used by model
// execution. Model metadata, pricing, capabilities, and client construction are
// separate concerns; this package owns only provider identity, configuration,
// provenance, and registry operations.
package provider

import (
	"context"
)

// Provider is one registry entry: a stable provider id plus the credentials a
// Run uses to build its client. The id also keys model reference data.
type Provider struct {
	// ID is the provider id — lowercase, e.g. "anthropic", "deepseek".
	ID string

	// APIKey is the raw provider key. It is sensitive and must never be logged or
	// exposed without masking.
	APIKey string

	// BaseURL optionally overrides the provider's default API endpoint.
	BaseURL string

	// KeySource is the provenance of APIKey — where the effective credential
	// came from. The bare registry leaves it zero ([KeyNone] / [KeyStored] is
	// derivable from APIKey); [WithEnvKeys] distinguishes a stored key from one
	// read from the environment. It is resolved per read rather than persisted.
	KeySource KeySource
}

// KeySource is where a provider's effective API key came from.
type KeySource string

const (
	// KeyNone — no key; the provider is unconfigured and not enabled.
	KeyNone KeySource = ""
	// KeyStored — key set via providers.update (persisted in the registry).
	KeyStored KeySource = "stored"
	// KeyEnv — key read from the provider's environment variable (not persisted;
	// surfaced by [WithEnvKeys]). A stored key always takes precedence.
	KeyEnv KeySource = "env"
)

// Enabled reports whether the provider is usable — i.e. it has an API key.
// A seeded-but-unconfigured provider is listed but not enabled until a key is set.
func (p Provider) Enabled() bool { return p.APIKey != "" }

// Patch is an atomic partial update to a provider's persisted configuration.
// A nil field preserves the stored value; a non-nil field replaces it,
// including replacing it with the empty string to clear that setting.
type Patch struct {
	APIKey  *string
	BaseURL *string
}

// Apply returns p with patch applied.
func (p Provider) Apply(patch Patch) Provider {
	if patch.APIKey != nil {
		p.APIKey = *patch.APIKey
	}
	if patch.BaseURL != nil {
		p.BaseURL = *patch.BaseURL
	}
	return p
}

// Empty reports whether patch preserves every persisted field.
func (patch Patch) Empty() bool {
	return patch.APIKey == nil && patch.BaseURL == nil
}

// Registry is the provider registry. All methods are safe for concurrent use.
type Registry interface {
	// List returns every known provider (the seeded supported set plus any
	// configured at runtime), enabled or not, sorted by ID.
	List(ctx context.Context) ([]Provider, error)

	// Get returns one provider by id; ok is false when unknown.
	Get(ctx context.Context, id string) (Provider, bool, error)

	// Update atomically applies patch to the provider identified by id, creating
	// an empty entry first when the id has not been persisted. It returns the
	// resulting persisted value; decorators may project effective credentials
	// onto that result without changing what is stored.
	Update(ctx context.Context, id string, patch Patch) (Provider, error)
}
