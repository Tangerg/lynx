package protocol

// TestProviderRequest identifies the configured provider to probe.
type TestProviderRequest struct {
	Provider string `json:"provider"`
}

// Provider is one configured LLM provider (API.md §4.9). The key is
// returned masked, never reconstructable.
type Provider struct {
	ID           string `json:"id"`
	BaseURL      string `json:"baseUrl,omitempty"`
	APIKeyMasked string `json:"apiKeyMasked"` // "" = unconfigured; e.g. "sk****78"
	// KeySource is the provenance of the key: "stored" (set via
	// providers.update, editable) or "env" (read from the provider's
	// environment variable, read-only — shown as "from env"). Omitted when the
	// provider is unconfigured (apiKeyMasked is also "").
	KeySource ProviderKeySource `json:"keySource,omitempty"`
	// RequiresBaseURL marks providers with no built-in endpoint — the generic
	// "openai-compatible" / "anthropic-compatible" passthroughs and Azure
	// (per-resource URL). The client must collect a base URL when configuring
	// them, and (since they carry no catalog) a free-form model id.
	RequiresBaseURL bool `json:"requiresBaseUrl,omitempty"`
	// EmbeddingCapable marks providers with an embeddings adapter — the set the
	// agent-memory embedding-role picker offers (models.setEmbeddingRole).
	// DefaultEmbeddingModel is a sensible default model id to prefill ("" when
	// the id is user-supplied, e.g. an Azure deployment).
	EmbeddingCapable      bool   `json:"embeddingCapable,omitempty"`
	DefaultEmbeddingModel string `json:"defaultEmbeddingModel,omitempty"`
}

// ProviderKeySource records where the visible API key originates. The empty
// value means no key is configured and is intentionally omitted on the wire.
type ProviderKeySource string

const (
	ProviderKeySourceStored ProviderKeySource = "stored"
	ProviderKeySourceEnv    ProviderKeySource = "env"
)

// ProviderConfigChangeType is the operation applied to one persisted provider
// setting. Omitting the enclosing request field preserves the stored value.
type ProviderConfigChangeType string

const (
	ProviderConfigSet   ProviderConfigChangeType = "set"
	ProviderConfigClear ProviderConfigChangeType = "clear"
)

// ProviderConfigChange is an explicit provider-setting mutation. Set requires
// a non-empty value; clear carries no value. Its tagged shape avoids overloading
// an empty string or JSON null with hidden update semantics.
type ProviderConfigChange struct {
	Type  ProviderConfigChangeType `json:"type"`
	Value *string                  `json:"value,omitempty"`
}

// UpdateProviderRequest — providers.update body. Provider is the
// provider id (Provider.id), e.g. "deepseek" — a meaningful slug, named to
// match the `provider` reference field elsewhere (Model.provider,
// runs.start), not "providerId".
// Omitted configuration fields are preserved; each present field explicitly
// sets or clears its stored value.
type UpdateProviderRequest struct {
	Provider string                `json:"provider"`
	BaseURL  *ProviderConfigChange `json:"baseUrl,omitempty"`
	APIKey   *ProviderConfigChange `json:"apiKey,omitempty"`
}

// ProviderTestResult — providers.test result.
type ProviderTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}
