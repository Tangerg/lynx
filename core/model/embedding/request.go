package embedding

import (
	"errors"
	"maps"

	"github.com/Tangerg/lynx/pkg/ptr"
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

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra,omitzero"`
}

// Returns an error
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

// ensureExtra must only be called by Set — Get must not mutate state
// because it is the concurrency-safe read path.
func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

func (o *Options) Get(key string) (any, bool) {
	if o == nil || o.Extra == nil {
		return nil, false
	}
	value, exists := o.Extra[key]
	return value, exists
}

func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
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
		Extra:          maps.Clone(o.Extra),
	}
}

// MergeOptions clones base and applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// Returns an error when base is nil.
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("embedding.MergeOptions: base options must not be nil")
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
	if src.EncodingFormat.Valid() {
		o.EncodingFormat = src.EncodingFormat
	}
	if src.Dimensions != nil {
		o.Dimensions = src.Dimensions
	}
	if len(src.Extra) > 0 {
		if o.Extra == nil {
			o.Extra = make(map[string]any, len(src.Extra))
		}
		maps.Copy(o.Extra, src.Extra)
	}
}

// Request is one embedding call: the input texts, the options, and
// caller-supplied side-channel params (user id, trace id, ...).
type Request struct {
	// Texts is the input list. Each entry produces one embedding.
	Texts []string `json:"texts,omitzero"`

	Options *Options `json:"options,omitempty"`

	// Params is per-request metadata middlewares can read.
	Params map[string]any `json:"params,omitzero"`
}

// Returns an error when texts
// is empty.
//
// Example:
//
//	req, err := embedding.NewRequest([]string{"hello", "world"})
func NewRequest(texts []string) (*Request, error) {
	if len(texts) == 0 {
		return nil, errors.New("embedding.NewRequest: texts must contain at least one entry")
	}
	return &Request{Texts: texts}, nil
}

func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

func (r *Request) Get(key string) (any, bool) {
	if r == nil || r.Params == nil {
		return nil, false
	}
	value, exists := r.Params[key]
	return value, exists
}

func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}
