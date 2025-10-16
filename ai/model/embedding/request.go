package embedding

import (
	"errors"
	"maps"

	"github.com/Tangerg/lynx/pkg/ptr"
)

// EncodingFormat specifies the format in which embedding vectors are encoded.
type EncodingFormat string

const (
	// EncodingFormatFloat represents embeddings as floating-point numbers.
	// This is the most common format for direct vector operations.
	EncodingFormatFloat EncodingFormat = "float"

	// EncodingFormatBase64 represents embeddings as base64-encoded strings.
	// This format is useful for compact transmission and storage.
	EncodingFormatBase64 EncodingFormat = "base64"
)

// Valid checks whether the encoding format is one of the supported formats.
// Returns true if the format is valid, false otherwise.
func (e EncodingFormat) Valid() bool {
	switch e {
	case EncodingFormatFloat,
		EncodingFormatBase64:
		return true
	default:
		return false
	}
}

// Options contains configuration parameters for embedding requests.
type Options struct {
	// Model The embedding model identifier to use
	Model string `json:"model"`

	// EncodingFormat specifies how the embedding vectors should be encoded in the response.
	EncodingFormat EncodingFormat `json:"encoding_format"`

	// Dimensions specifies the desired dimensionality of the output embeddings.
	// If nil, the model's default dimensions will be used.
	Dimensions *int64 `json:"dimensions"`

	// Extra holds provider-specific options that are not part of the standard options.
	// This allows for extensibility without modifying the core Options structure.
	Extra map[string]any `json:"extra"`
}

func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("model cannot be empty")
	}

	return &Options{
		Model: model,
	}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

func (o *Options) Get(key string) (any, bool) {
	o.ensureExtra()
	value, exists := o.Extra[key]
	return value, exists
}

func (o *Options) Set(key string, value any) {
	o.ensureExtra()
	o.Extra[key] = value
}

// Clone creates a deep copy of the Options.
// Returns nil if the original Options is nil.
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

func MergeOptions(options *Options, opts ...*Options) (*Options, error) {
	if options == nil {
		return nil, errors.New("options cannot be nil")
	}

	mergedOpts := options.Clone()

	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if opt.Model != "" {
			mergedOpts.Model = opt.Model
		}
		if opt.EncodingFormat.Valid() {
			mergedOpts.EncodingFormat = opt.EncodingFormat
		}
		if opt.Dimensions != nil {
			mergedOpts.Dimensions = opt.Dimensions
		}
		if len(opt.Extra) > 0 {
			maps.Copy(mergedOpts.Extra, opt.Extra)
		}
	}

	return mergedOpts, nil
}

// Request represents an embedding generation request containing input texts and configuration.
type Request struct {
	// Inputs contains the text strings to be converted into embeddings.
	Inputs []string `json:"inputs"`

	// Options specifies the configuration for how embeddings should be generated.
	Options *Options `json:"options"`

	// Params holds request-level metadata and parameters such as userID, sessionID, etc.
	// These parameters typically don't affect the embedding generation but are useful for tracking and logging.
	Params map[string]any `json:"params"`
}

// NewRequest creates a new embedding request with the given input texts.
// Returns an error if the inputs slice is empty, as at least one input is required.
func NewRequest(inputs []string) (*Request, error) {
	if len(inputs) == 0 {
		return nil, errors.New("inputs cannot be empty: at least one input string is required")
	}

	return &Request{
		Inputs: inputs,
		Params: make(map[string]any),
	}, nil
}

func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

func (r *Request) Get(key string) (any, bool) {
	r.ensureParams()
	value, exists := r.Params[key]
	return value, exists
}

func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}
