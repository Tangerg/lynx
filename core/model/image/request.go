package image

import (
	"errors"
	"maps"

	"github.com/Tangerg/lynx/pkg/mime"
	"github.com/Tangerg/lynx/pkg/ptr"
)

// ResponseFormat selects how a provider returns a generated image —
// either a URL pointing at hosted bytes, or base64-encoded bytes inline.
type ResponseFormat string

const (
	// ResponseFormatURL returns a URL the caller can fetch.
	ResponseFormatURL ResponseFormat = "url"

	// ResponseFormatB64JSON returns the image bytes inline as base64.
	ResponseFormatB64JSON ResponseFormat = "b64json"
)

func (r ResponseFormat) String() string { return string(r) }

// Valid reports whether r is one of the recognized formats.
func (r ResponseFormat) Valid() bool {
	switch r {
	case ResponseFormatURL, ResponseFormatB64JSON:
		return true
	default:
		return false
	}
}

// Options holds per-request configuration for an image-generation call.
// Pointer fields use nil to mean "not set" — providers fall back to
// their own defaults.
type Options struct {
	// Model is the provider model identifier (e.g. "dall-e-3").
	Model string `json:"model"`

	// NegativePrompt describes what should not appear in the image.
	NegativePrompt string `json:"negative_prompt"`

	// Width / Height set the output dimensions in pixels.
	Width  *int64 `json:"width,omitempty"`
	Height *int64 `json:"height,omitempty"`

	// Style selects the artistic style (provider-specific values).
	Style string `json:"style"`

	// Quality controls render quality (e.g. DALL-E 3 "standard" / "hd",
	// gpt-image-1 "low" / "medium" / "high" / "auto", Stability
	// "low" / "medium" / "high"). Provider-specific values; empty leaves
	// the choice to the provider.
	Quality string `json:"quality"`

	// Seed pins the RNG so repeated calls produce the same image.
	Seed *int64 `json:"seed,omitempty"`

	// OutputFormat picks the MIME type of the rendered bytes.
	OutputFormat *mime.MIME `json:"output_format,omitempty"`

	// ResponseFormat picks URL vs inline base64.
	ResponseFormat ResponseFormat `json:"response_format"`

	// Extra carries provider-specific options unknown to this struct.
	Extra map[string]any `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("image.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

func (o *Options) ensureExtra() {
	if o.Extra == nil {
		o.Extra = make(map[string]any)
	}
}

// Get returns the Extra value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (o *Options) Get(key string) (any, bool) {
	if o.Extra == nil {
		return nil, false
	}
	value, exists := o.Extra[key]
	return value, exists
}

// Set stores value under key in Extra.
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
		NegativePrompt: o.NegativePrompt,
		Width:          ptr.Clone(o.Width),
		Height:         ptr.Clone(o.Height),
		Style:          o.Style,
		Quality:        o.Quality,
		Seed:           ptr.Clone(o.Seed),
		OutputFormat:   o.OutputFormat.Clone(),
		ResponseFormat: o.ResponseFormat,
		Extra:          maps.Clone(o.Extra),
	}
}

// MergeOptions clones base then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// Returns an error when base is nil.
func MergeOptions(base *Options, overrides ...*Options) (*Options, error) {
	if base == nil {
		return nil, errors.New("image.MergeOptions: base options must not be nil")
	}

	merged := base.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		applyOverride(merged, override)
	}
	return merged, nil
}

// applyOverride mutates dst in place with the non-zero fields of src.
func applyOverride(dst, src *Options) {
	if src.NegativePrompt != "" {
		dst.NegativePrompt = src.NegativePrompt
	}
	if src.Model != "" {
		dst.Model = src.Model
	}
	if src.Width != nil {
		dst.Width = src.Width
	}
	if src.Height != nil {
		dst.Height = src.Height
	}
	if src.Style != "" {
		dst.Style = src.Style
	}
	if src.Quality != "" {
		dst.Quality = src.Quality
	}
	if src.Seed != nil {
		dst.Seed = src.Seed
	}
	if src.OutputFormat != nil {
		dst.OutputFormat = src.OutputFormat
	}
	if src.ResponseFormat.Valid() {
		dst.ResponseFormat = src.ResponseFormat
	}
	if len(src.Extra) > 0 {
		if dst.Extra == nil {
			dst.Extra = make(map[string]any, len(src.Extra))
		}
		maps.Copy(dst.Extra, src.Extra)
	}
}

// Request is one image-generation call: the prompt, options, and
// caller-supplied side-channel params.
type Request struct {
	// Prompt is the natural-language description of the desired image.
	Prompt string `json:"prompt"`

	// Options carries model-specific parameters.
	Options *Options `json:"options,omitempty"`

	// Params is per-request metadata middlewares can read.
	Params map[string]any `json:"params,omitzero"`
}

// NewRequest builds a Request from prompt. Returns an error when prompt
// is empty.
func NewRequest(prompt string) (*Request, error) {
	if prompt == "" {
		return nil, errors.New("image.NewRequest: prompt must not be empty")
	}
	return &Request{Prompt: prompt}, nil
}

func (r *Request) ensureParams() {
	if r.Params == nil {
		r.Params = make(map[string]any)
	}
}

// Get returns the Params value for key plus an existence flag. See
// [chat.Options.Get] for the concurrency contract.
func (r *Request) Get(key string) (any, bool) {
	if r.Params == nil {
		return nil, false
	}
	value, exists := r.Params[key]
	return value, exists
}

// Set stores value under key in Params.
func (r *Request) Set(key string, value any) {
	r.ensureParams()
	r.Params[key] = value
}
