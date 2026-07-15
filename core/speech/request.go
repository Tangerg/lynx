package speech

import (
	"errors"
	"fmt"
	"maps"

	"github.com/Tangerg/lynx/core/metadata"
)

// Options holds per-request configuration for a TTS call.
type Options struct {
	// Model is the provider model identifier (e.g. "tts-1").
	Model string `json:"model"`

	// Voice selects the speaker profile. Provider-specific values.
	Voice string `json:"voice"`

	// ResponseFormat selects the audio container ("mp3", "wav", ...).
	ResponseFormat string `json:"response_format"`

	// Speed scales the playback rate. 1.0 is normal speed.
	Speed float64 `json:"speed"`

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("tts.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return errors.New("speech.Options.Set: nil receiver")
	}
	return setExtra(&o.Extra, key, value)
}

func setExtra(target *metadata.Map, key string, value any) error {
	if *target != nil {
		return metadata.Set(*target, key, value)
	}
	candidate := metadata.New()
	if err := metadata.Set(candidate, key, value); err != nil {
		return err
	}
	*target = candidate
	return nil
}

func (o *Options) validate() error {
	if o == nil {
		return nil
	}
	if o.Speed < 0 {
		return errors.New("speech: speed must not be negative")
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
		Model:          o.Model,
		Voice:          o.Voice,
		ResponseFormat: o.ResponseFormat,
		Speed:          o.Speed,
		Extra:          o.Extra.Clone(),
	}
}

// MergeOptions clones base then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// Returns an error when base is nil.
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("tts.MergeOptions: base options must not be nil")
	}

	merged := base.Clone()
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
		if override.ResponseFormat != "" {
			merged.ResponseFormat = override.ResponseFormat
		}
		if override.Speed != 0 {
			merged.Speed = override.Speed
		}
		if len(override.Extra) > 0 {
			if merged.Extra == nil {
				merged.Extra = metadata.New()
			}
			maps.Copy(merged.Extra, override.Extra.Clone())
		}
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
