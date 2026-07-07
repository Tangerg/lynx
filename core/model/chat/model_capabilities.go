package chat

import "slices"

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
