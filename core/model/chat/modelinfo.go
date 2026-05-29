package chat

import "slices"

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
	// --- identity ---

	// ID is the model identifier (e.g. "claude-sonnet-4-6").
	ID string `json:"id,omitempty"`

	// DisplayName is a human-readable label (e.g. "Claude Sonnet 4.6").
	DisplayName string `json:"display_name,omitempty"`

	// KnowledgeCutoff is the training knowledge cutoff (e.g. "2025-05"),
	// empty when unknown.
	KnowledgeCutoff string `json:"knowledge_cutoff,omitempty"`

	// --- pricing ---

	// Pricing is the per-1M-token rate card, zero (IsZero) when unknown.
	Pricing Pricing `json:"pricing,omitzero"`

	// --- capabilities ---

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

	// --- limits ---

	// ContextWindow is the maximum total context size in tokens
	// (0 = unknown).
	ContextWindow int64 `json:"context_window,omitempty"`

	// MaxInputTokens is the maximum prompt size in tokens, when the
	// provider caps it separately from the context window (0 = unknown).
	MaxInputTokens int64 `json:"max_input_tokens,omitempty"`

	// MaxOutputTokens is the maximum completion size in tokens
	// (0 = unknown).
	MaxOutputTokens int64 `json:"max_output_tokens,omitempty"`
}

// IsZero reports whether no model info is set. (Spelled out rather than
// `m == ModelInfo{}` because nested slices make ModelInfo non-comparable.)
func (m ModelInfo) IsZero() bool {
	return m.ID == "" && m.DisplayName == "" && m.KnowledgeCutoff == "" &&
		m.Pricing.IsZero() && m.Reasoning.IsZero() && m.Modalities.IsZero() &&
		!m.ToolCall && !m.StructuredOutput &&
		m.ContextWindow == 0 && m.MaxInputTokens == 0 && m.MaxOutputTokens == 0
}

// Pricing is a chat model's per-token rate card, expressed (like the
// industry convention) as USD per 1,000,000 tokens. It lives in the chat
// package because input/output/cache token rates are a chat-model concept
// — other modalities price differently (per image, per second of audio),
// so this isn't a generic model type.
//
// The zero value means "unknown" — treat a zero Pricing as "no cost
// available", not "free". Surface it via [ModelInfo.Pricing] so cost can
// be attributed without the consumer hard-coding a rate table. The rate
// table itself (which model costs what) is reference data that tracks
// vendor pricing and lives outside this protocol layer (the models module
// ships one sourced from models.dev).
type Pricing struct {
	// InputPer1M is the rate for uncached prompt (input) tokens.
	InputPer1M float64 `json:"input_per_1m"`

	// OutputPer1M is the rate for completion (output) tokens.
	OutputPer1M float64 `json:"output_per_1m"`

	// CacheReadPer1M is the (discounted) rate for prompt tokens served
	// from the provider's prompt cache. Zero falls back to InputPer1M.
	CacheReadPer1M float64 `json:"cache_read_per_1m,omitempty"`

	// CacheWritePer1M is the (premium) rate for prompt tokens written to
	// the provider's prompt cache. Zero falls back to InputPer1M.
	CacheWritePer1M float64 `json:"cache_write_per_1m,omitempty"`
}

// IsZero reports whether the rate card is unset (all rates zero) — i.e.
// pricing is unknown for this model.
func (p Pricing) IsZero() bool {
	return p == Pricing{}
}

// Cost computes the USD cost of one call from a [Usage] breakdown.
//
// [Usage.PromptTokens] is the FULL prompt count and already includes any
// cache-read / cache-write portions (per the Usage docs), so those are
// billed at their own rates and subtracted from the uncached remainder.
// A nil/zero cache rate falls back to the standard input rate so a model
// that reports cache tokens but lists no cache rate isn't under-billed.
func (p Pricing) Cost(u *Usage) float64 {
	if u == nil || p.IsZero() {
		return 0
	}
	cacheRead := derefInt64(u.CacheReadInputTokens)
	cacheWrite := derefInt64(u.CacheWriteInputTokens)
	uncachedIn := max(u.PromptTokens-cacheRead-cacheWrite, 0)

	readRate := p.CacheReadPer1M
	if readRate == 0 {
		readRate = p.InputPer1M
	}
	writeRate := p.CacheWritePer1M
	if writeRate == 0 {
		writeRate = p.InputPer1M
	}

	total := float64(uncachedIn)*p.InputPer1M +
		float64(u.CompletionTokens)*p.OutputPer1M +
		float64(cacheRead)*readRate +
		float64(cacheWrite)*writeRate
	return total / 1_000_000
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// Reasoning describes a chat model's extended-thinking (reasoning)
// support. Like [Pricing], it's a value type with an [IsZero] check and
// nests inside [ModelInfo].
//
// Supported is the authoritative "can this model reason" bit. Levels and
// DefaultLevel are populated only when effort is level-controlled (e.g.
// OpenAI's "low"/"medium"/"high", Gemini's tiers); a model that reasons
// via a token budget (Anthropic) reports Supported with no Levels.
type Reasoning struct {
	// Supported reports whether the model can reason / think at all.
	Supported bool `json:"supported,omitempty"`

	// Levels are the discrete effort levels the model accepts, in
	// increasing order (e.g. "low", "medium", "high"). Nil when effort
	// isn't level-controlled.
	Levels []string `json:"levels,omitempty"`

	// DefaultLevel is the effort used when the caller doesn't pick one.
	// Empty when there are no Levels.
	DefaultLevel string `json:"default_level,omitempty"`
}

// IsZero reports whether reasoning is unset — i.e. the model can't reason.
func (r Reasoning) IsZero() bool {
	return !r.Supported && len(r.Levels) == 0 && r.DefaultLevel == ""
}

// Modality is a media type a model takes as input or produces as output.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
	ModalityPDF   Modality = "pdf"
)

// Modalities lists the media a model accepts as input and emits as output,
// mirroring how providers describe a model card (Gemini's "input source /
// output", OpenAI's input types, Anthropic's image_input / pdf_input
// capabilities). Text is listed explicitly even though every chat model
// accepts it, so each list is self-describing. Like [Pricing] /
// [Reasoning], it's a value type with an [IsZero] check.
type Modalities struct {
	// Input is the media the model accepts, text first then richer types
	// (e.g. text, image, pdf, audio, video).
	Input []Modality `json:"input,omitempty"`

	// Output is the media the model emits — text for chat models.
	Output []Modality `json:"output,omitempty"`
}

// IsZero reports whether no modalities are known.
func (m Modalities) IsZero() bool {
	return len(m.Input) == 0 && len(m.Output) == 0
}

// AcceptsInput reports whether the model takes the given input modality.
func (m Modalities) AcceptsInput(x Modality) bool {
	return slices.Contains(m.Input, x)
}

// EmitsOutput reports whether the model produces the given output modality.
func (m Modalities) EmitsOutput(x Modality) bool {
	return slices.Contains(m.Output, x)
}
