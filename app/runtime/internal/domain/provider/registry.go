// Package provider is Lyra's LLM-provider registry — the runtime-mutable
// set of providers Lyra can talk to, each carrying the credentials a turn
// needs to build a client. It's the credentials + enablement layer; the
// per-model metadata (which models a provider offers, pricing, capabilities)
// is reference data read from the models module's catalog, not stored here.
//
// The registry is seeded at startup with the supported provider ids (so
// providers.list always reports the full supported set), and a provider
// becomes "enabled" once it has an API key — set from config at boot or via
// providers.configure at runtime. The SQLite registry keeps runtime edits
// across restarts.
//
// Deliberately a data-only registry (credentials + enablement + CRUD): model
// metadata/pricing live in the models catalog and per-turn client construction
// in the composition root, so there is no richer domain behavior to add — keep
// domain logic out of this registry.
package provider

import (
	"context"
)

// Provider is one registry entry: a provider id plus the credentials a turn
// uses to build its client. The id doubles as the adapter type Lyra resolves
// (anthropic / openai / moonshot / deepseek) and the key into the models
// catalog.
type Provider struct {
	// ID is the provider id — lowercase, e.g. "anthropic", "deepseek".
	ID string

	// APIKey is the raw provider key. Stored as-is (the registry persists
	// it like config.yaml does); masked at the wire boundary, never logged.
	APIKey string

	// BaseURL optionally overrides the provider's default API endpoint.
	BaseURL string

	// KeySource is the provenance of APIKey — where the effective credential
	// came from. The bare registry leaves it zero ([KeyNone] / [KeyStored] is
	// derivable from APIKey); the env-fallback decorator ([WithEnvKeys]) is what
	// distinguishes a stored key from one read from the environment, so the wire
	// can show "from env". Not persisted — it's resolved per read.
	KeySource KeySource
}

// Metadata is static provider reference data: whether this runtime build knows
// how to talk to the provider, which UI affordances it needs, and whether it can
// serve embeddings. Credentials and enablement remain in [Provider].
type Metadata struct {
	ID                    string
	RequiresBaseURL       bool
	EmbeddingCapable      bool
	DefaultEmbeddingModel string
	// ProbeModels marks a provider whose available models are defined by its live
	// endpoint, not the static catalog — models.list probes /v1/models for these
	// (local / bring-your-own-endpoint providers whose model id is user-supplied).
	ProbeModels bool
}

// KeySource is where a provider's effective API key came from. It rides on the
// wire (Provider.keySource) so the UI can tell a stored key (set via
// providers.configure, editable + persisted) from one picked up from the
// environment (read-only, shown as "from env").
type KeySource string

const (
	// KeyNone — no key; the provider is unconfigured and not enabled.
	KeyNone KeySource = ""
	// KeyStored — key set via providers.configure (persisted in the registry).
	KeyStored KeySource = "stored"
	// KeyEnv — key read from the provider's environment variable (not persisted;
	// surfaced by [WithEnvKeys]). A stored key always takes precedence.
	KeyEnv KeySource = "env"
)

// Enabled reports whether the provider is usable — i.e. it has an API key.
// A seeded-but-unconfigured provider is listed (so the UI can offer it) but
// not enabled until a key is set.
func (p Provider) Enabled() bool { return p.APIKey != "" }

// Registry is the provider registry. All methods are safe for concurrent use.
type Registry interface {
	// List returns every known provider (the seeded supported set plus any
	// configured at runtime), enabled or not, sorted by ID.
	List(ctx context.Context) ([]Provider, error)

	// Get returns one provider by id; ok is false when unknown.
	Get(ctx context.Context, id string) (Provider, bool, error)

	// Configure upserts a provider's credentials (key + base URL) by ID,
	// persisting the change. Used both to seed at startup and to apply a
	// runtime providers.configure.
	Configure(ctx context.Context, p Provider) error
}
