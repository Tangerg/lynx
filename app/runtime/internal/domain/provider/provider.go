// Package provider defines model-provider identity, credential configuration,
// effective key provenance, and patch semantics. Registry persistence,
// environment lookup, model metadata, pricing, and client construction remain
// outside this package.
package provider

// Provider is one registry entry: a stable provider id plus the credentials
// used for model access. The id also keys model reference data.
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
	// KeyStored — key persisted in the registry.
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

// Empty reports whether p preserves every persisted field.
func (p Patch) Empty() bool {
	return p.APIKey == nil && p.BaseURL == nil
}
