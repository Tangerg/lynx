package transcription

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
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

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error when
// model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("transcription.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return errors.New("transcription.Options.Set: nil receiver")
	}
	return o.Extra.Set(key, value)
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
		Temperature:          clonePointer(o.Temperature),
		ResponseFormat:       o.ResponseFormat,
		TimestampGranularity: slices.Clone(o.TimestampGranularity),
		Extra:                o.Extra.Clone(),
	}
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
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
		o.Temperature = clonePointer(src.Temperature)
	}
	if src.ResponseFormat != "" {
		o.ResponseFormat = src.ResponseFormat
	}
	if len(src.TimestampGranularity) > 0 {
		o.TimestampGranularity = slices.Clone(src.TimestampGranularity)
	}
	if len(src.Extra) > 0 {
		if o.Extra == nil {
			o.Extra = metadata.New()
		}
		maps.Copy(o.Extra, src.Extra.Clone())
	}
}

func (o *Options) validate() error {
	if o == nil {
		return nil
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("transcription: options extra: %w", err)
	}
	return nil
}

// Request is one transcription call: the audio payload and explicit options.
type Request struct {
	// Audio carries the audio bytes (or URL) to transcribe.
	Audio *media.Media `json:"audio,omitempty"`

	Options *Options `json:"options,omitempty"`
}

// NewRequest builds a Request from an audio payload. Returns an error
// when audio is nil.
func NewRequest(audio *media.Media) (*Request, error) {
	r := &Request{Audio: audio}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("transcription.NewRequest: %w", err)
	}
	return r, nil
}

// Validate checks the complete request before it crosses a model boundary.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("transcription: nil request")
	}
	if r.Audio == nil {
		return errors.New("transcription: audio must not be nil")
	}
	if err := r.Audio.Validate(); err != nil {
		return fmt.Errorf("transcription: audio: %w", err)
	}
	return r.Options.validate()
}
