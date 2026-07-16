package moderation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

// Options holds per-request configuration for a moderation call.
type Options struct {
	// Model is the provider model identifier.
	Model string `json:"model"`

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty or has surrounding whitespace.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, fmt.Errorf("moderation.NewOptions: %w: model id must not be empty", ErrInvalidOptions)
	}
	if strings.TrimSpace(model) != model {
		return nil, fmt.Errorf("moderation.NewOptions: %w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return fmt.Errorf("moderation.Options.Set: %w: nil receiver", ErrInvalidOptions)
	}
	if err := o.Extra.Set(key, value); err != nil {
		return fmt.Errorf("moderation.Options.Set: %w: %w", ErrInvalidOptions, err)
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
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: extra: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Clone returns a deep copy. nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Model: o.Model,
		Extra: o.Extra.Clone(),
	}
}

// Merged clones o then applies each override left-to-right.
// A nil receiver returns an error.
func (o *Options) Merged(overrides ...*Options) (*Options, error) {
	if o == nil {
		return nil, fmt.Errorf("moderation.Options.Merged: %w: nil receiver", ErrInvalidOptions)
	}

	merged := o.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		if override.Model != "" {
			merged.Model = override.Model
		}
		if len(override.Extra) > 0 {
			if err := merged.Extra.Merge(override.Extra); err != nil {
				return nil, fmt.Errorf("moderation.Options.Merged: %w: merge Extra: %w", ErrInvalidOptions, err)
			}
		}
	}
	if err := merged.validate(); err != nil {
		return nil, fmt.Errorf("moderation.Options.Merged: %w", err)
	}
	return merged, nil
}

// Request is one moderation call: the input texts and explicit options.
type Request struct {
	// Texts is the input list. Each entry is moderated independently.
	Texts []string `json:"texts,omitzero"`

	Options *Options `json:"options,omitempty"`
}

// NewRequest builds a Request from texts. Returns an error when texts
// is empty.
func NewRequest(texts []string) (*Request, error) {
	r := &Request{Texts: slices.Clone(texts)}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("moderation.NewRequest: %w", err)
	}
	return r, nil
}

// Validate checks the complete request before it crosses a model boundary.
func (r *Request) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil request", ErrInvalidRequest)
	}
	if len(r.Texts) == 0 {
		return fmt.Errorf("%w: texts must contain at least one entry", ErrInvalidRequest)
	}
	for i, text := range r.Texts {
		if text == "" {
			return fmt.Errorf("%w: texts[%d] must not be empty", ErrInvalidRequest, i)
		}
	}
	if err := r.Options.validate(); err != nil {
		return fmt.Errorf("%w: options: %w", ErrInvalidRequest, err)
	}
	return nil
}
