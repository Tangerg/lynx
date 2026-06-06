// Package provider is Lyra's LLM-provider registry — the runtime-mutable
// set of providers Lyra can talk to, each carrying the credentials a turn
// needs to build a client. It's the credentials + enablement layer; the
// per-model metadata (which models a provider offers, pricing, capabilities)
// is reference data read from the models module's catalog, not stored here.
//
// The registry is seeded at startup with the supported provider ids (so
// providers.list always reports the full supported set), and a provider
// becomes "enabled" once it has an API key — set from config at boot or via
// providers.configure at runtime. Persisted backends (file / sqlite) keep
// runtime edits across restarts.
package provider

import "context"

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
}

// Enabled reports whether the provider is usable — i.e. it has an API key.
// A seeded-but-unconfigured provider is listed (so the UI can offer it) but
// not enabled until a key is set.
func (p Provider) Enabled() bool { return p.APIKey != "" }

// MaskedAPIKey renders the key for the wire (API.md §4.9 apiKeyMasked): "" for
// an unconfigured provider (the disabled signal), a fixed redaction for short
// keys, otherwise an ellipsis + the last four chars (e.g. "…fc78"). The
// provider owns how to present its own secret, so the wire boundary never
// touches the raw key. This is distinct from the log-safe [core/model.APIKey]
// masking — that guards against accidental leakage in logs/JSON, this is the
// deliberate UI form.
func (p Provider) MaskedAPIKey() string {
	switch {
	case p.APIKey == "":
		return ""
	case len(p.APIKey) <= 8:
		return "••••"
	default:
		return "…" + p.APIKey[len(p.APIKey)-4:]
	}
}

// Service is the provider registry. All methods are safe for concurrent use.
type Service interface {
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
