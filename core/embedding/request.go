package embedding

import (
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/internal/extension"
	"github.com/Tangerg/lynx/core/internal/ptr"
	"github.com/Tangerg/lynx/core/metadata"
)

// Options holds per-request configuration for an embedding call. Pointer
// fields use nil to mean "not set" — providers fall back to their own
// defaults.
type Options struct {
	// Model is the provider model identifier
	// (e.g. "text-embedding-3-small").
	Model string `json:"model"`

	// Dimensions requests an explicit output vector size. nil leaves it
	// up to the provider's default.
	Dimensions *int64 `json:"dimensions,omitempty"`

	// Extensions carries JSON-safe provider-specific options unknown to this
	// struct.
	Extensions metadata.Map `json:"extensions,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty or has surrounding whitespace.
//
// Example:
//
//	opts, err := embedding.NewOptions("text-embedding-3-small")
func NewOptions(model string) (Options, error) {
	if model == "" {
		return Options{}, fmt.Errorf("embedding.NewOptions: %w: model id must not be empty", ErrInvalidOptions)
	}
	if strings.TrimSpace(model) != model {
		return Options{}, fmt.Errorf("embedding.NewOptions: %w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	return Options{Model: model}, nil
}

// SetExtension encodes a provider-specific option under a namespace/name key.
func (o *Options) SetExtension(key string, value any) error {
	if o == nil {
		return fmt.Errorf("embedding.Options.SetExtension: %w: nil receiver", ErrInvalidOptions)
	}
	if err := extension.Set(&o.Extensions, key, value); err != nil {
		return fmt.Errorf("embedding.Options.SetExtension: %w: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Clone returns a deep copy of o.
func (o Options) Clone() Options {
	return Options{
		Model:      o.Model,
		Dimensions: ptr.Clone(o.Dimensions),
		Extensions: o.Extensions.Clone(),
	}
}

// Merged clones o and applies each override left-to-right.
// Scalar non-zero values overwrite; the Extensions map merges last-write-wins.
func (o Options) Merged(overrides ...Options) (Options, error) {
	merged := o.Clone()
	for _, override := range overrides {
		if err := merged.applyOverride(override); err != nil {
			return Options{}, fmt.Errorf("embedding.Options.Merged: %w: %w", ErrInvalidOptions, err)
		}
	}
	if err := merged.Validate(); err != nil {
		return Options{}, fmt.Errorf("embedding.Options.Merged: %w", err)
	}
	return merged, nil
}

func (o *Options) applyOverride(src Options) error {
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.Dimensions != nil {
		o.Dimensions = ptr.Clone(src.Dimensions)
	}
	if len(src.Extensions) > 0 {
		if err := o.Extensions.Merge(src.Extensions); err != nil {
			return fmt.Errorf("merge extensions: %w", err)
		}
	}
	return nil
}

// Validate verifies explicitly supplied overrides. Options{} is valid.
func (o Options) Validate() error {
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return fmt.Errorf("%w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	if o.Dimensions != nil && *o.Dimensions <= 0 {
		return fmt.Errorf("%w: dimensions must be positive", ErrInvalidOptions)
	}
	if err := extension.Validate(o.Extensions); err != nil {
		return fmt.Errorf("%w: extensions: %w", ErrInvalidOptions, err)
	}
	return nil
}

// Request is one embedding call: the input texts and explicit options.
type Request struct {
	// Texts is the input list. Each entry produces one embedding.
	Texts []string `json:"texts,omitzero"`

	Options Options `json:"options,omitzero"`
}

// NewRequest builds a Request from texts. Returns an error when texts
// is empty.
//
// Example:
//
//	req, err := embedding.NewRequest([]string{"hello", "world"})
func NewRequest(texts []string) (*Request, error) {
	r := &Request{Texts: slices.Clone(texts)}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("embedding.NewRequest: %w", err)
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
