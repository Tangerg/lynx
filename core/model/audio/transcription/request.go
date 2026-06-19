package transcription

import (
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/pkg/ptr"
)

// Options holds per-request configuration for a transcription call.
// Pointer fields use nil to mean "not set" — providers fall back to
// their own defaults.
type Options struct {
	// Model is the provider model identifier (e.g. "whisper-1").
	Model string `json:"model"`

	// Language is an ISO-639-1 language code (e.g. "en", "zh") hinting
	// the spoken language. Empty leaves detection to the provider.
	// OpenAI / Deepgram / AssemblyAI / Gladia / Rev AI / ElevenLabs all
	// accept this.
	Language string `json:"language"`

	// Prompt biases the model toward expected vocabulary or formatting.
	// On Whisper this is the "previous-context" string used for
	// terminology hints; provider semantics vary but the field is
	// almost always called "prompt" or maps onto a vocab-hint field.
	Prompt string `json:"prompt"`

	// Temperature controls sampling randomness (Whisper / Whisper-large
	// variants). Range typically 0.0–1.0. nil leaves it to the provider.
	Temperature *float64 `json:"temperature,omitempty"`

	// ResponseFormat selects the transcript shape. Common values: "json",
	// "verbose_json", "text", "srt", "vtt". Provider-specific values
	// pass through verbatim.
	ResponseFormat string `json:"response_format"`

	// TimestampGranularity selects timestamp resolution. OpenAI accepts
	// "word" and / or "segment"; other providers may accept "utterance"
	// etc. Empty leaves the choice to the provider.
	TimestampGranularity []string `json:"timestamp_granularity,omitzero"`

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra,omitzero"`
}

// NewOptions Returns an error
// when model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("transcription.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

func (o *Options) Get(key string) (any, bool) {
	if o == nil || o.Extra == nil {
		return nil, false
	}
	value, exists := o.Extra[key]
	return value, exists
}

func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
}

// Clone returns a deep copy. nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Model:                o.Model,
		Language:             o.Language,
		Prompt:               o.Prompt,
		Temperature:          ptr.Clone(o.Temperature),
		ResponseFormat:       o.ResponseFormat,
		TimestampGranularity: slices.Clone(o.TimestampGranularity),
		Extra:                maps.Clone(o.Extra),
	}
}

// MergeOptions clones base then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// Returns an error when base is nil.
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("transcription.MergeOptions: base options must not be nil")
	}

	merged := base.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		merged.applyOverride(override)
	}
	return merged, nil
}

func (o *Options) applyOverride(src *Options) {
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.Language != "" {
		o.Language = src.Language
	}
	if src.Prompt != "" {
		o.Prompt = src.Prompt
	}
	if src.Temperature != nil {
		o.Temperature = src.Temperature
	}
	if src.ResponseFormat != "" {
		o.ResponseFormat = src.ResponseFormat
	}
	if len(src.TimestampGranularity) > 0 {
		o.TimestampGranularity = slices.Clone(src.TimestampGranularity)
	}
	if len(src.Extra) > 0 {
		if o.Extra == nil {
			o.Extra = make(map[string]any, len(src.Extra))
		}
		maps.Copy(o.Extra, src.Extra)
	}
}

// Request is one transcription call: the audio payload, options, and
// caller-supplied side-channel params.
type Request struct {
	// Audio carries the audio bytes (or URL) to transcribe.
	Audio *media.Media `json:"audio,omitempty"`

	Options *Options `json:"options,omitempty"`

	// Params is per-request metadata middlewares can read.
	Params map[string]any `json:"params,omitzero"`
}

// NewRequest builds a Request from an audio payload. Returns an error
// when audio is nil.
func NewRequest(audio *media.Media) (*Request, error) {
	if audio == nil {
		return nil, errors.New("transcription.NewRequest: audio must not be nil")
	}
	return &Request{Audio: audio}, nil
}
