package image

import (
	"errors"
	"maps"

	"github.com/Tangerg/lynx/pkg/mime"
	"github.com/Tangerg/lynx/pkg/ptr"
)

// ResponseFormat defines the format of the response from image generation APIs
type ResponseFormat string

const (
	// ResponseFormatURL indicates the response will contain a URL to the generated image
	ResponseFormatURL ResponseFormat = "url"
	// ResponseFormatB64JSON indicates the response will contain base64-encoded JSON data
	ResponseFormatB64JSON ResponseFormat = "b64json"
)

// String returns the string representation of ResponseFormat
func (r ResponseFormat) String() string {
	return string(r)
}

// Valid checks if the ResponseFormat is a valid value
func (r ResponseFormat) Valid() bool {
	switch r {
	case ResponseFormatURL,
		ResponseFormatB64JSON:
		return true
	default:
		return false
	}
}

// Options represents the configuration options for image generation
type Options struct {
	// NegativePrompt specifies what should not appear in the generated image
	NegativePrompt string `json:"negative_prompt"`

	// Model specifies the AI model to use for image generation
	Model string `json:"model"`

	// Width specifies the width of the generated image in pixels
	Width *int64 `json:"width"`

	// Height specifies the height of the generated image in pixels
	Height *int64 `json:"height"`

	// Style specifies the artistic style of the generated image
	Style string `json:"style"`

	// Seed is used for reproducible random generation
	Seed *int64 `json:"seed"`

	// OutputFormat specifies the MIME type of the output image
	OutputFormat *mime.MIME `json:"output_format"`

	// ResponseFormat specifies how the response should be formatted
	ResponseFormat ResponseFormat `json:"response_format"`

	// Extra holds additional custom parameters
	Extra map[string]any `json:"extra"`
}

// NewOptions creates a new Options instance with the specified model
// Returns an error if the model is empty
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("no model provided")
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

// Clone creates a deep copy of the Options object
// Returns nil if the original Options is nil
func (o *Options) Clone() *Options {
	if o == nil {
		return nil
	}
	return &Options{
		NegativePrompt: o.NegativePrompt,
		Model:          o.Model,
		Width:          ptr.Clone(o.Width),
		Height:         ptr.Clone(o.Height),
		Style:          o.Style,
		Seed:           ptr.Clone(o.Seed),
		OutputFormat:   o.OutputFormat.Clone(),
		ResponseFormat: o.ResponseFormat,
		Extra:          maps.Clone(o.Extra),
	}
}

// MergeOptions merges multiple Options into a single Options instance.
// It creates a clone of the base options and applies each subsequent option in order.
// Later options override earlier ones for all fields.
//
// Parameters:
//   - options: The base Options to clone (must not be nil)
//   - opts: Additional Options to merge (nil entries are skipped)
//
// Returns:
//   - *Options: The merged result
//   - error: An error if the base options is nil
//
// Merge behavior:
//   - String fields (NegativePrompt, Model, Style): Later non-empty values override earlier ones
//   - Pointer fields (Width, Height, Seed, OutputFormat): Later non-nil values override earlier ones
//   - ResponseFormat field: Later valid values override earlier ones
func MergeOptions(options *Options, opts ...*Options) (*Options, error) {
	if options == nil {
		return nil, errors.New("options cannot be nil")
	}
	mergedOpts := options.Clone()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if opt.NegativePrompt != "" {
			mergedOpts.NegativePrompt = opt.NegativePrompt
		}
		if opt.Model != "" {
			mergedOpts.Model = opt.Model
		}
		if opt.Width != nil {
			mergedOpts.Width = opt.Width
		}
		if opt.Height != nil {
			mergedOpts.Height = opt.Height
		}
		if opt.Style != "" {
			mergedOpts.Style = opt.Style
		}
		if opt.Seed != nil {
			mergedOpts.Seed = opt.Seed
		}
		if opt.OutputFormat != nil {
			mergedOpts.OutputFormat = opt.OutputFormat
		}
		if opt.ResponseFormat.Valid() {
			mergedOpts.ResponseFormat = opt.ResponseFormat
		}
		if len(opt.Extra) > 0 {
			maps.Copy(mergedOpts.Extra, opt.Extra)
		}
	}

	return mergedOpts, nil
}

// Request represents an image generation request
type Request struct {
	// Prompt is the text description of the desired image
	Prompt string `json:"prompt"`

	// Options contains the generation configuration
	Options *Options `json:"options"`

	// Params holds additional request parameters
	Params map[string]any `json:"params"`
}

// NewRequest creates a new Request instance with the specified prompt
// Returns an error if the prompt is empty
func NewRequest(prompt string) (*Request, error) {
	if prompt == "" {
		return nil, errors.New("prompt cannot be empty")
	}
	return &Request{
		Prompt: prompt,
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
