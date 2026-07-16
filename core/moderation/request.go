package moderation

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/internal/extension"
	"github.com/Tangerg/lynx/core/metadata"
)

// Options holds per-request configuration for a moderation call.
type Options struct {
	// Model is the provider model identifier.
	Model string `json:"model"`

	// Extensions carries JSON-safe provider-specific options unknown to this
	// struct.
	Extensions metadata.Map `json:"extensions,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty or has surrounding whitespace.
func NewOptions(model string) (Options, error) {
	if model == "" {
		return Options{}, fmt.Errorf("moderation.NewOptions: %w: model id must not be empty", ErrInvalidOptions)
	}
	if strings.TrimSpace(model) != model {
		return Options{}, fmt.Errorf("moderation.NewOptions: %w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	return Options{Model: model}, nil
}

// SetExtension encodes a provider-specific option under a namespace/name key.
func (o *Options) SetExtension(key string, value any) error {
	if o == nil {
		return fmt.Errorf("moderation.Options.SetExtension: %w: nil receiver", ErrInvalidOptions)
	}
	if err := extension.Set(&o.Extensions, key, value); err != nil {
		return fmt.Errorf("moderation.Options.SetExtension: %w: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Validate verifies explicitly supplied overrides. Options{} is valid.
func (o Options) Validate() error {
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return fmt.Errorf("%w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	if err := extension.Validate(o.Extensions); err != nil {
		return fmt.Errorf("%w: extensions: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Clone returns a deep copy of o.
func (o Options) Clone() Options {
	return Options{
		Model:      o.Model,
		Extensions: o.Extensions.Clone(),
	}
}

// Merged clones o then applies each override left-to-right.
func (o Options) Merged(overrides ...Options) (Options, error) {
	merged := o.Clone()
	for _, override := range overrides {
		if override.Model != "" {
			merged.Model = override.Model
		}
		if len(override.Extensions) > 0 {
			if err := merged.Extensions.Merge(override.Extensions); err != nil {
				return Options{}, fmt.Errorf("moderation.Options.Merged: %w: merge extensions: %w", ErrInvalidOptions, err)
			}
		}
	}
	if err := merged.Validate(); err != nil {
		return Options{}, fmt.Errorf("moderation.Options.Merged: %w", err)
	}
	return merged, nil
}

// Request is one moderation call: the input texts and explicit options.
type Request struct {
	// Texts is the input list. Each entry is moderated independently.
	Texts []string `json:"texts,omitzero"`

	Options Options `json:"options,omitzero"`
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
	if err := r.Options.Validate(); err != nil {
		return fmt.Errorf("%w: options: %w", ErrInvalidRequest, err)
	}
	return nil
}
