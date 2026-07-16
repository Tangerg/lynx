package transcription

import (
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

// Options holds the provider-neutral configuration shared by transcription
// implementations. Provider-specific controls belong in Extra.
type Options struct {
	// Model is the provider model identifier (e.g. "whisper-1").
	Model string `json:"model"`

	// Language is an ISO-639-1 language code (e.g. "en", "zh") hinting
	// the spoken language. Empty leaves detection to the provider.
	Language string `json:"language"`

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error when
// model is empty or has surrounding whitespace.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, fmt.Errorf("transcription.NewOptions: %w: model id must not be empty", ErrInvalidOptions)
	}
	if strings.TrimSpace(model) != model {
		return nil, fmt.Errorf("transcription.NewOptions: %w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return fmt.Errorf("transcription.Options.Set: %w: nil receiver", ErrInvalidOptions)
	}
	if err := o.Extra.Set(key, value); err != nil {
		return fmt.Errorf("transcription.Options.Set: %w: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Clone returns a deep copy. nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Model:    o.Model,
		Language: o.Language,
		Extra:    o.Extra.Clone(),
	}
}

// Merged clones o then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// A nil receiver returns an error.
func (o *Options) Merged(overrides ...*Options) (*Options, error) {
	if o == nil {
		return nil, fmt.Errorf("transcription.Options.Merged: %w: nil receiver", ErrInvalidOptions)
	}

	merged := o.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		if err := merged.applyOverride(override); err != nil {
			return nil, fmt.Errorf("transcription.Options.Merged: %w: %w", ErrInvalidOptions, err)
		}
	}
	if err := merged.validate(); err != nil {
		return nil, fmt.Errorf("transcription.Options.Merged: %w", err)
	}
	return merged, nil
}

func (o *Options) applyOverride(src *Options) error {
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.Language != "" {
		o.Language = src.Language
	}
	if len(src.Extra) > 0 {
		if err := o.Extra.Merge(src.Extra); err != nil {
			return fmt.Errorf("merge Extra: %w", err)
		}
	}
	return nil
}

func (o *Options) validate() error {
	if o == nil {
		return nil
	}
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return fmt.Errorf("%w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	if o.Language != "" && strings.TrimSpace(o.Language) != o.Language {
		return fmt.Errorf("%w: language must not have surrounding whitespace", ErrInvalidOptions)
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: extra: %w", ErrInvalidOptions, err)
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
		return fmt.Errorf("%w: nil request", ErrInvalidRequest)
	}
	if r.Audio == nil {
		return fmt.Errorf("%w: audio must not be nil", ErrInvalidRequest)
	}
	if err := r.Audio.Validate(); err != nil {
		return fmt.Errorf("%w: audio: %w", ErrInvalidRequest, err)
	}
	if err := r.Options.validate(); err != nil {
		return fmt.Errorf("%w: options: %w", ErrInvalidRequest, err)
	}
	return nil
}
