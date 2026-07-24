package protocol

import "context"

// Providers is the providers.* method group (API.md §7.6).
type Providers interface {
	ListProviders(ctx context.Context, q PageQuery) (*Page[Provider], error)
	ConfigureProvider(ctx context.Context, in ConfigureProviderRequest) (*Provider, error)
	TestProvider(ctx context.Context, providerID string) (*ProviderTestResult, error)
}

// TestProviderRequest identifies the configured provider to probe.
type TestProviderRequest struct {
	Provider string `json:"provider"`
}

// Models is the models.* method group.
type Models interface {
	ListModels(ctx context.Context, in ListModelsRequest) (*Page[Model], error)
	// GetUtilityRole reports the (provider, model) the in-house maintenance
	// work (compaction / extraction / titling) runs on — empty model when
	// unset (it runs on the main turn model).
	GetUtilityRole(ctx context.Context) (*UtilityRole, error)
	// SetUtilityRole points that maintenance work at a (provider, model),
	// validated by resolving the client; an empty model clears it back to the
	// main turn model. Persisted across restarts.
	SetUtilityRole(ctx context.Context, in UtilityRole) (*UtilityRole, error)
	// GetEmbeddingRole reports the (provider, model) the @codebase semantic index
	// embeds with — empty model when unset (the index feature is off).
	GetEmbeddingRole(ctx context.Context) (*EmbeddingRole, error)
	// SetEmbeddingRole points the index at an (embedding-capable provider, model),
	// validated by building the embedding client; an empty model clears it. A new
	// model invalidates a project's vectors (re-embedded on next use). Persisted.
	SetEmbeddingRole(ctx context.Context, in EmbeddingRole) (*EmbeddingRole, error)
}

// UtilityRole is the (provider, model) the in-house maintenance services run
// on (models.getUtilityRole / setUtilityRole). Empty model = unset → those run
// on the main turn model. Provider must be a configured provider id.
type UtilityRole struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// EmbeddingRole is the (provider, model) the @codebase semantic index embeds
// with (models.getEmbeddingRole / setEmbeddingRole). Empty model = unset → the
// index feature is off. Provider must be a configured, embedding-capable
// provider. A distinct type from [UtilityRole] (same shape, different domain —
// under the rule-of-three for sharing).
type EmbeddingRole struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// ListModelsRequest — models.list body (API.md §7.6). Provider is optional
// (models are organized by provider; omitted → empty page); PageQuery paginates.
type ListModelsRequest struct {
	Provider string `json:"provider,omitempty"`
	PageQuery
}

// Provider is one configured LLM provider (API.md §4.9). The key is
// returned masked, never reconstructable.
type Provider struct {
	ID           string `json:"id"`
	BaseURL      string `json:"baseUrl,omitempty"`
	APIKeyMasked string `json:"apiKeyMasked"` // "" = unconfigured; e.g. "sk****78"
	// KeySource is the provenance of the key: "stored" (set via
	// providers.configure, editable) or "env" (read from the provider's
	// environment variable, read-only — shown as "from env"). Omitted when the
	// provider is unconfigured (apiKeyMasked is also "").
	KeySource ProviderKeySource `json:"keySource,omitempty"`
	// RequiresBaseURL marks providers with no built-in endpoint — the generic
	// "openai-compatible" / "anthropic-compatible" passthroughs and Azure
	// (per-resource URL). The client must collect a base URL when configuring
	// them, and (since they carry no catalog) a free-form model id.
	RequiresBaseURL bool `json:"requiresBaseUrl,omitempty"`
	// EmbeddingCapable marks providers with an embeddings adapter — the set the
	// @codebase embedding-role picker offers (models.setEmbeddingRole).
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

// ConfigureProviderRequest — providers.configure body. Provider is the
// provider id (Provider.id), e.g. "deepseek" — a meaningful slug, named to
// match the `provider` reference field elsewhere (Model.provider,
// runs.start), not "providerId".
type ConfigureProviderRequest struct {
	Provider string `json:"provider"`
	BaseURL  string `json:"baseUrl,omitempty"`
	APIKey   string `json:"apiKey,omitempty"`
}

// ProviderTestResult — providers.test result.
type ProviderTestResult struct {
	OK    bool         `json:"ok"`
	Error *ProblemData `json:"error,omitempty"`
}

// Model is one entry in models.list (API.md §4.9).
type Model struct {
	ID              string `json:"id"`
	Provider        string `json:"provider"`
	DisplayName     string `json:"displayName,omitempty"`
	ContextWindow   int    `json:"contextWindow,omitempty"`
	MaxInputTokens  int    `json:"maxInputTokens,omitempty"`
	MaxOutputTokens int    `json:"maxOutputTokens,omitempty"`
	// KnowledgeCutoff is the training cutoff (RFC3339 date), empty when unknown.
	KnowledgeCutoff string `json:"knowledgeCutoff,omitempty"`
	// Deprecated marks a model the provider has retired; clients hide or flag it.
	Deprecated   bool               `json:"deprecated,omitempty"`
	Capabilities *ModelCapabilities `json:"capabilities,omitempty"`
	Pricing      *ModelPricing      `json:"pricing,omitempty"`
}

// Modality is a media type a model takes as input or emits as output
// (API.md §4.9), mirroring core's chat.Modality. Open enum — new media types
// are added without bumping the contract.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
	ModalityPDF   Modality = "pdf"
)

// ModelCapabilities — per-model capabilities (API.md §4.9). The booleans are
// quick gates; the list / level fields carry the detail a model picker needs
// (which media the model accepts, what reasoning effort levels it offers).
type ModelCapabilities struct {
	// Reasoning reports whether the model supports extended thinking at all.
	Reasoning bool `json:"reasoning,omitempty"`
	// ReasoningLevels are the discrete effort levels the model accepts, in
	// increasing order (e.g. ["low","medium","high"]). Empty when reasoning is
	// budget-controlled (no discrete levels) or unsupported.
	ReasoningLevels []string `json:"reasoningLevels,omitempty"`
	// ReasoningDefaultLevel is the effort used when the caller picks none;
	// empty when there are no levels.
	ReasoningDefaultLevel string `json:"reasoningDefaultLevel,omitempty"`
	// Multimodal is a convenience flag: the model accepts image input. The
	// full set is InputModalities.
	Multimodal bool `json:"multimodal,omitempty"`
	// InputModalities lists every media type the model accepts (text first,
	// then richer types). OutputModalities lists what it emits (text for chat).
	InputModalities  []Modality `json:"inputModalities,omitempty"`
	OutputModalities []Modality `json:"outputModalities,omitempty"`
	// ToolUse reports tool / function calling support.
	ToolUse bool `json:"toolUse,omitempty"`
	// StructuredOutput reports native structured-output / JSON-schema support.
	StructuredOutput bool `json:"structuredOutput,omitempty"`
}

// ModelPricing — per-million-token pricing (API.md §4.9). The primary rate
// band; cache rates are zero when the provider doesn't price cache separately.
// Long-context models that reprice past a token threshold carry only their base
// band here — full banded pricing isn't surfaced on the wire.
type ModelPricing struct {
	InputUsdPerMillionTokens      float64 `json:"inputUsdPerMillionTokens,omitempty"`
	OutputUsdPerMillionTokens     float64 `json:"outputUsdPerMillionTokens,omitempty"`
	CacheReadUsdPerMillionTokens  float64 `json:"cacheReadUsdPerMillionTokens,omitempty"`
	CacheWriteUsdPerMillionTokens float64 `json:"cacheWriteUsdPerMillionTokens,omitempty"`
}
