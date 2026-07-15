package embedding

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/lynx/core/internal/ptr"
	"github.com/Tangerg/lynx/core/metadata"
)

// EncodingFormat enumerates the on-the-wire shapes a provider may use
// for embedding vectors. Most callers want [EncodingFormatFloat] —
// [EncodingFormatBase64] is useful when transmitting compactly over
// channels that re-encode binary data.
type EncodingFormat string

const (
	// EncodingFormatFloat encodes each vector as JSON float numbers.
	EncodingFormatFloat EncodingFormat = "float"

	// EncodingFormatBase64 encodes each vector as a base64 string of the
	// little-endian float32 byte sequence.
	EncodingFormatBase64 EncodingFormat = "base64"
)

func (e EncodingFormat) Valid() bool {
	switch e {
	case EncodingFormatFloat, EncodingFormatBase64:
		return true
	default:
		return false
	}
}

// Options holds per-request configuration for an embedding call. Pointer
// fields use nil to mean "not set" — providers fall back to their own
// defaults.
type Options struct {
	// Model is the provider model identifier
	// (e.g. "text-embedding-3-small").
	Model string `json:"model"`

	// EncodingFormat picks the wire shape the provider should return.
	EncodingFormat EncodingFormat `json:"encoding_format"`

	// Dimensions requests an explicit output vector size. nil leaves it
	// up to the provider's default.
	Dimensions *int64 `json:"dimensions,omitempty"`

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty.
//
// Example:
//
//	opts, err := embedding.NewOptions("text-embedding-3-small")
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("embedding.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return errors.New("embedding.Options.Set: nil receiver")
	}
	return o.Extra.Set(key, value)
}

// Clone returns a deep copy. nil receiver yields nil.
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		Model:          o.Model,
		EncodingFormat: o.EncodingFormat,
		Dimensions:     ptr.Clone(o.Dimensions),
		Extra:          o.Extra.Clone(),
	}
}

// Merged clones o and applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// A nil receiver returns an error.
func (o *Options) Merged(overrides ...*Options) (*Options, error) {
	if o == nil {
		return nil, errors.New("embedding.Options.Merged: nil receiver")
	}

	merged := o.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		if err := merged.applyOverride(override); err != nil {
			return nil, fmt.Errorf("embedding.Options.Merged: %w", err)
		}
	}
	return merged, nil
}

func (o *Options) applyOverride(src *Options) error {
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.EncodingFormat.Valid() {
		o.EncodingFormat = src.EncodingFormat
	}
	if src.Dimensions != nil {
		o.Dimensions = ptr.Clone(src.Dimensions)
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
	if o.EncodingFormat != "" && !o.EncodingFormat.Valid() {
		return fmt.Errorf("embedding: invalid encoding format %q", o.EncodingFormat)
	}
	if o.Dimensions != nil && *o.Dimensions <= 0 {
		return errors.New("embedding: dimensions must be positive")
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("embedding: options extra: %w", err)
	}
	return nil
}

// Request is one embedding call: the input texts and explicit options.
type Request struct {
	// Texts is the input list. Each entry produces one embedding.
	Texts []string `json:"texts,omitzero"`

	Options *Options `json:"options,omitempty"`
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
		return errors.New("embedding: nil request")
	}
	if len(r.Texts) == 0 {
		return errors.New("embedding: texts must contain at least one entry")
	}
	for i, text := range r.Texts {
		if text == "" {
			return fmt.Errorf("embedding: texts[%d] must not be empty", i)
		}
	}
	return r.Options.validate()
}
