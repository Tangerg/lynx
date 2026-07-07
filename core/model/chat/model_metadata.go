package chat

import "time"

// ModelMetadata holds identity metadata for a [Model] instance: the
// vendor plus the instance's default model info. Provider names are
// conventionally lowercase ("openai", "anthropic", ...) so downstream
// filters match without case folding.
//
// Per-model metadata for arbitrary model ids lives in a catalog keyed by
// model id (the models module ships one sourced from models.dev); Model
// is just the default model's view.
type ModelMetadata struct {
	// Provider names the LLM vendor — "openai", "anthropic", "google", etc.
	Provider string `json:"provider"`

	// Model is the instance's default model metadata — pricing,
	// capabilities, and identity. Accessed as meta.Model.Pricing etc.
	Model ModelInfo `json:"model,omitzero"`
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

	// KnowledgeCutoff is the training knowledge cutoff, zero when unknown.
	// Month-precision sources land on the first of the month.
	KnowledgeCutoff time.Time `json:"knowledge_cutoff,omitzero"`

	// ReleaseDate is the model's first public release date, zero when
	// unknown.
	ReleaseDate time.Time `json:"release_date,omitzero"`

	// LastUpdated is when the model spec last changed, zero when unknown.
	LastUpdated time.Time `json:"last_updated,omitzero"`

	// Deprecated reports whether the provider has retired the model. It's
	// kept in the catalog — cost attribution still works for callers on
	// the old id — so consumers can hide or flag it rather than lose it.
	Deprecated bool `json:"deprecated,omitempty"`

	// Pricing is the per-1M-token rate card as one or more bands (see
	// [Pricing]) — usually a single band, more for long-context models
	// that reprice past a token threshold. Nil when unknown. Use [CostOf]
	// to price a call across the bands.
	Pricing []Pricing `json:"pricing,omitempty"`

	// Reasoning describes extended-thinking support, zero (IsZero) when
	// the model can't reason.
	Reasoning Reasoning `json:"reasoning,omitzero"`

	// Modalities lists the media the model takes as input and emits as
	// output, zero (IsZero) when unknown.
	Modalities Modalities `json:"modalities,omitzero"`

	// ToolCall reports whether the model supports tool / function calling.
	ToolCall bool `json:"tool_call,omitempty"`

	// StructuredOutput reports whether the model supports a native
	// structured-output / JSON-schema feature.
	StructuredOutput bool `json:"structured_output,omitempty"`

	// Limits is the model's token limits (context window, max input /
	// output), zero (IsZero) when unknown.
	Limits Limits `json:"limits,omitzero"`
}

// IsZero reports whether no model info is set. (Spelled out rather than
// `m == ModelInfo{}` because nested slices make ModelInfo non-comparable.)
func (m ModelInfo) IsZero() bool {
	return m.ID == "" && m.DisplayName == "" && m.KnowledgeCutoff.IsZero() &&
		m.ReleaseDate.IsZero() && m.LastUpdated.IsZero() && !m.Deprecated &&
		len(m.Pricing) == 0 && m.Reasoning.IsZero() && m.Modalities.IsZero() &&
		!m.ToolCall && !m.StructuredOutput && m.Limits.IsZero()
}
