package chat

import (
	"slices"
	"time"

	"github.com/Tangerg/lynx/pkg/ptr"
)

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

// Pricing is one band of a chat model's per-token rate card, expressed
// (like the industry convention) as USD per 1,000,000 tokens. A model's
// full rate card is a []Pricing (see [ModelInfo.Pricing]): usually a
// single band, but long-context models carry more — a base band plus
// higher bands that take over once the prompt crosses [Pricing.Threshold]
// (Gemini 2.5 Pro is $1.25/$10 up to 200K input tokens, $2.50/$15 beyond;
// several OpenAI models split short vs long context the same way).
//
// IMPORTANT: a band reprices the WHOLE prompt, not the marginal tokens
// above the threshold — a 250K-token Gemini 2.5 Pro call is billed
// entirely at the >200K rates, not 200K at base + 50K at the tier. See
// [CostOf] for selection across bands; [Pricing.Cost] is one band alone.
//
// It lives in the chat package because input/output/cache token rates are
// a chat-model concept — other modalities price differently (per image,
// per second of audio). The rate table itself (which model costs what) is
// reference data outside this protocol layer (the models module ships one
// sourced from models.dev).
type Pricing struct {
	// Threshold is the prompt input-token count at or above which this
	// band applies. 0 is the base band (always applicable).
	Threshold int64 `json:"threshold,omitempty"`

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

func (p Pricing) IsZero() bool { return p == Pricing{} }

// CostOf computes the USD cost of one call from a [Usage] breakdown,
// across a banded rate card (see [Pricing]). The prompt's input-token size
// selects the band — the highest [Pricing.Threshold] the prompt reaches,
// scanning back to front (bands are ascending by Threshold) — and the
// whole call bills at that band. Returns 0 for an empty card or nil usage.
func CostOf(bands []Pricing, u *Usage) float64 {
	if u == nil || len(bands) == 0 {
		return 0
	}
	band := bands[0]
	for i := len(bands) - 1; i >= 0; i-- {
		if u.PromptTokens >= bands[i].Threshold {
			band = bands[i]
			break
		}
	}
	return band.Cost(u)
}

// Cost computes the USD cost of one call at THIS band's rates (no band
// selection — see [CostOf] for that).
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
	cacheRead := ptr.From(u.CacheReadInputTokens)
	cacheWrite := ptr.From(u.CacheWriteInputTokens)
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

// Limits is a chat model's token limits. Like [Pricing] / [Reasoning] /
// [Modalities], it's a value type with an [IsZero] check and nests inside
// [ModelInfo]. A zero field means "unknown", not "unlimited".
type Limits struct {
	// ContextWindow is the maximum total context size in tokens.
	ContextWindow int64 `json:"context_window,omitempty"`

	// MaxInputTokens is the maximum prompt size in tokens, when the
	// provider caps it separately from the context window.
	MaxInputTokens int64 `json:"max_input_tokens,omitempty"`

	// MaxOutputTokens is the maximum completion size in tokens.
	MaxOutputTokens int64 `json:"max_output_tokens,omitempty"`
}

func (l Limits) IsZero() bool { return l == Limits{} }

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

func (m Modalities) IsZero() bool {
	return len(m.Input) == 0 && len(m.Output) == 0
}

func (m Modalities) AcceptsInput(x Modality) bool {
	return slices.Contains(m.Input, x)
}

func (m Modalities) EmitsOutput(x Modality) bool {
	return slices.Contains(m.Output, x)
}
