package speech

import (
	"fmt"
	"math"
	"strings"

	"github.com/Tangerg/lynx/core/internal/extension"
	"github.com/Tangerg/lynx/core/metadata"
)

// Options holds per-request configuration for a TTS call.
type Options struct {
	// Model is the provider model identifier (e.g. "tts-1").
	Model string `json:"model"`

	// Voice selects the speaker profile. Provider-specific values.
	Voice string `json:"voice"`

	// OutputFormat selects the audio container ("mp3", "wav", ...).
	OutputFormat string `json:"output_format"`

	// Speed scales the playback rate. 1.0 is normal speed.
	Speed float64 `json:"speed"`

	// Extensions carries JSON-safe provider-specific options unknown to this
	// struct.
	Extensions metadata.Map `json:"extensions,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty or has surrounding whitespace.
func NewOptions(model string) (Options, error) {
	if model == "" {
		return Options{}, fmt.Errorf("speech.NewOptions: %w: model id must not be empty", ErrInvalidOptions)
	}
	if strings.TrimSpace(model) != model {
		return Options{}, fmt.Errorf("speech.NewOptions: %w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	return Options{Model: model}, nil
}

// SetExtension encodes a provider-specific option under a namespace/name key.
func (o *Options) SetExtension(key string, value any) error {
	if o == nil {
		return fmt.Errorf("speech.Options.SetExtension: %w: nil receiver", ErrInvalidOptions)
	}
	if err := extension.Set(&o.Extensions, key, value); err != nil {
		return fmt.Errorf("speech.Options.SetExtension: %w: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Validate verifies explicitly supplied overrides. Options{} is valid.
func (o Options) Validate() error {
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return fmt.Errorf("%w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	if math.IsNaN(o.Speed) || math.IsInf(o.Speed, 0) || o.Speed < 0 {
		return fmt.Errorf("%w: speed must be finite and non-negative", ErrInvalidOptions)
	}
	if err := extension.Validate(o.Extensions); err != nil {
		return fmt.Errorf("%w: extensions: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Clone returns a deep copy of o.
func (o Options) Clone() Options {
	return Options{
		Model:        o.Model,
		Voice:        o.Voice,
		OutputFormat: o.OutputFormat,
		Speed:        o.Speed,
		Extensions:   o.Extensions.Clone(),
	}
}

// Merged clones o then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extensions map merges last-write-wins.
func (o Options) Merged(overrides ...Options) (Options, error) {
	merged := o.Clone()
	for _, override := range overrides {
		if override.Model != "" {
			merged.Model = override.Model
		}
		if override.Voice != "" {
			merged.Voice = override.Voice
		}
		if override.OutputFormat != "" {
			merged.OutputFormat = override.OutputFormat
		}
		if override.Speed != 0 {
			merged.Speed = override.Speed
		}
		if len(override.Extensions) > 0 {
			if err := merged.Extensions.Merge(override.Extensions); err != nil {
				return Options{}, fmt.Errorf("speech.Options.Merged: %w: merge extensions: %w", ErrInvalidOptions, err)
			}
		}
	}
	if err := merged.Validate(); err != nil {
		return Options{}, fmt.Errorf("speech.Options.Merged: %w", err)
	}
	return merged, nil
}

// Request is one TTS call: the input text and explicit options.
type Request struct {
	// Text is the prompt converted to speech.
	Text string `json:"text"`

	Options Options `json:"options,omitzero"`
}

// NewRequest builds a Request from text. Returns an error when text
// is empty.
func NewRequest(text string) (*Request, error) {
	r := &Request{Text: text}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("speech.NewRequest: %w", err)
	}
	return r, nil
}

// Validate checks the complete request before it crosses a model boundary.
func (r *Request) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil request", ErrInvalidRequest)
	}
	if r.Text == "" {
		return fmt.Errorf("%w: text must not be empty", ErrInvalidRequest)
	}
	if err := r.Options.Validate(); err != nil {
		return fmt.Errorf("%w: options: %w", ErrInvalidRequest, err)
	}
	return nil
}
