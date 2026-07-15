package speech

import (
	"errors"
	"fmt"
	"math"
	"strings"

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

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty or has surrounding whitespace.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("speech.NewOptions: model id must not be empty")
	}
	if strings.TrimSpace(model) != model {
		return nil, errors.New("speech.NewOptions: model id must not have surrounding whitespace")
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return errors.New("speech.Options.Set: nil receiver")
	}
	return o.Extra.Set(key, value)
}

func (o *Options) validate() error {
	if o == nil {
		return nil
	}
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return errors.New("speech: model id must not have surrounding whitespace")
	}
	if math.IsNaN(o.Speed) || math.IsInf(o.Speed, 0) || o.Speed < 0 {
		return errors.New("speech: speed must be finite and non-negative")
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("speech: options extra: %w", err)
	}
	return nil
}

// Clone returns a deep copy. nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Model:        o.Model,
		Voice:        o.Voice,
		OutputFormat: o.OutputFormat,
		Speed:        o.Speed,
		Extra:        o.Extra.Clone(),
	}
}

// Merged clones o then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// A nil receiver returns an error.
func (o *Options) Merged(overrides ...*Options) (*Options, error) {
	if o == nil {
		return nil, errors.New("speech.Options.Merged: nil receiver")
	}

	merged := o.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
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
		if len(override.Extra) > 0 {
			if err := merged.Extra.Merge(override.Extra); err != nil {
				return nil, fmt.Errorf("speech.Options.Merged: merge Extra: %w", err)
			}
		}
	}
	if err := merged.validate(); err != nil {
		return nil, fmt.Errorf("speech.Options.Merged: %w", err)
	}
	return merged, nil
}

// Request is one TTS call: the input text and explicit options.
type Request struct {
	// Text is the prompt converted to speech.
	Text string `json:"text"`

	Options *Options `json:"options,omitempty"`
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
		return errors.New("speech: nil request")
	}
	if r.Text == "" {
		return errors.New("speech: text must not be empty")
	}
	return r.Options.validate()
}
