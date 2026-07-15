package moderation

import (
	"errors"
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
		return nil, errors.New("moderation.NewOptions: model id must not be empty")
	}
	if strings.TrimSpace(model) != model {
		return nil, errors.New("moderation.NewOptions: model id must not have surrounding whitespace")
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return errors.New("moderation.Options.Set: nil receiver")
	}
	return o.Extra.Set(key, value)
}

func (o *Options) validate() error {
	if o == nil {
		return nil
	}
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return errors.New("moderation: model id must not have surrounding whitespace")
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("moderation: options extra: %w", err)
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
		return nil, errors.New("moderation.Options.Merged: nil receiver")
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
				return nil, fmt.Errorf("moderation.Options.Merged: merge Extra: %w", err)
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
		return errors.New("moderation: nil request")
	}
	if len(r.Texts) == 0 {
		return errors.New("moderation: texts must contain at least one entry")
	}
	for i, text := range r.Texts {
		if text == "" {
			return fmt.Errorf("moderation: texts[%d] must not be empty", i)
		}
	}
	return r.Options.validate()
}
