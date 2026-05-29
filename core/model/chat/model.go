package chat

import (
	"github.com/Tangerg/lynx/core/model"
)

// Model is the provider surface: a chat LLM that supports both synchronous
// [model.Model.Call] and streaming [model.StreamingModel.Stream]. Implementations
// expose model-specific defaults via [Model.DefaultOptions] and an identity
// hint via [Model.Metadata] so callers and observability layers can branch on
// provider.
//
// Example:
//
//	type myModel struct{ /* ... */ }
//	func (m *myModel) Call(ctx context.Context, req *chat.Request) (*chat.Response, error) { ... }
//	func (m *myModel) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] { ... }
//	func (m *myModel) DefaultOptions() chat.Options { opts, _ := chat.NewOptions("gpt-4o"); return *opts }
//	func (m *myModel) Metadata() chat.ModelMetadata          { return chat.ModelMetadata{Provider: "openai"} }
//
//	var _ chat.Model = (*myModel)(nil)
type Model interface {
	model.Model[*Request, *Response]
	model.StreamingModel[*Request, *Response]

	// DefaultOptions returns the parameter set this provider uses when the
	// caller does not override anything. The returned value is a fresh copy
	// the caller may mutate.
	DefaultOptions() Options

	// Metadata returns identity metadata used by logging, metrics, and any
	// observability layer that needs to tag a span by provider.
	Metadata() ModelMetadata
}

// ModelInfo is a single chat model's metadata — identity, pricing, and
// capabilities. It's the row type of a model catalog (provider → many
// models) and the per-model payload carried by [ModelMetadata].
//
// A chat [Model] instance can serve more than one model id (the id is a
// request option), so the catalog — keyed by model id — is the canonical
// source; ModelMetadata carries the instance's default model's info as a
// convenience.
type ModelInfo struct {
	// ID is the model identifier (e.g. "claude-sonnet-4-6").
	ID string `json:"id,omitempty"`

	// DisplayName is a human-readable label (e.g. "Claude Sonnet 4.6").
	DisplayName string `json:"display_name,omitempty"`

	// Pricing is the per-1M-token rate card, zero (IsZero) when unknown.
	Pricing Pricing `json:"pricing,omitzero"`

	// Reasoning describes extended-thinking support, zero (IsZero) when
	// the model can't reason.
	Reasoning Reasoning `json:"reasoning,omitzero"`

	// ContextWindow is the maximum context size in tokens (0 = unknown).
	ContextWindow int64 `json:"context_window,omitempty"`

	// MaxTokens is the model's default max output tokens (0 = unknown).
	MaxTokens int64 `json:"max_tokens,omitempty"`
}

// IsZero reports whether no model info is set. (Spelled out rather than
// `m == ModelInfo{}` because Reasoning.Levels is a slice, so ModelInfo
// isn't comparable.)
func (m ModelInfo) IsZero() bool {
	return m.ID == "" && m.DisplayName == "" && m.Pricing.IsZero() &&
		m.Reasoning.IsZero() && m.ContextWindow == 0 && m.MaxTokens == 0
}

// ModelMetadata holds identity metadata for a [Model] instance: the
// vendor plus the instance's default model info. Provider names are
// conventionally lowercase ("openai", "anthropic", ...) so downstream
// filters match without case folding.
//
// Per-model metadata for arbitrary model ids lives in a catalog
// (github.com/Tangerg/lynx/models/catalog, modeled on catwalk); Model is
// just the default model's view.
type ModelMetadata struct {
	// Provider names the LLM vendor — "openai", "anthropic", "google", etc.
	Provider string `json:"provider"`

	// Model is the instance's default model metadata — pricing,
	// capabilities, and identity. Accessed as meta.Model.Pricing etc.
	Model ModelInfo `json:"model,omitzero"`
}
