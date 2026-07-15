package image

import (
	"errors"
	"fmt"
	"maps"
	"mime"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
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

	// OutputFormat picks the image MIME type of the rendered bytes.
	// Empty leaves the format to the provider.
	OutputFormat string `json:"output_format,omitempty"`

	// ResponseFormat picks URL vs inline base64.
	ResponseFormat ResponseFormat `json:"response_format"`

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, errors.New("image.NewOptions: model id must not be empty")
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return errors.New("image.Options.Set: nil receiver")
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
		NegativePrompt: o.NegativePrompt,
		Width:          clonePointer(o.Width),
		Height:         clonePointer(o.Height),
		Style:          o.Style,
		Quality:        o.Quality,
		Seed:           clonePointer(o.Seed),
		OutputFormat:   o.OutputFormat,
		ResponseFormat: o.ResponseFormat,
		Extra:          o.Extra.Clone(),
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
		merged.applyOverride(override)
	}
	normalized, err := normalizeOutputFormat(merged.OutputFormat)
	if err != nil {
		return nil, err
	}
	merged.OutputFormat = normalized
	return merged, nil
}

func normalizeOutputFormat(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err != nil {
		return "", fmt.Errorf("image.MergeOptions: invalid OutputFormat %q: %w", value, err)
	}
	mediaType = strings.ToLower(mediaType)
	if !strings.HasPrefix(mediaType, "image/") || len(strings.TrimPrefix(mediaType, "image/")) == 0 {
		return "", fmt.Errorf("image.MergeOptions: OutputFormat %q is not an image MIME type", value)
	}
	if len(parameters) != 0 {
		return "", fmt.Errorf("image.MergeOptions: OutputFormat %q must not include parameters", value)
	}
	return mediaType, nil
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	return new(*value)
}

func (o *Options) applyOverride(src *Options) {
	if src.NegativePrompt != "" {
		o.NegativePrompt = src.NegativePrompt
	}
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.Width != nil {
		o.Width = clonePointer(src.Width)
	}
	if src.Height != nil {
		o.Height = clonePointer(src.Height)
	}
	if src.Style != "" {
		o.Style = src.Style
	}
	if src.Quality != "" {
		o.Quality = src.Quality
	}
	if src.Seed != nil {
		o.Seed = clonePointer(src.Seed)
	}
	if src.OutputFormat != "" {
		o.OutputFormat = src.OutputFormat
	}
	if src.ResponseFormat.Valid() {
		o.ResponseFormat = src.ResponseFormat
	}
	if len(src.Extra) > 0 {
		if o.Extra == nil {
			o.Extra = metadata.New()
		}
		maps.Copy(o.Extra, src.Extra.Clone())
	}
}

func (o *Options) validate() error {
	if o == nil {
		return nil
	}
	if o.Width != nil && *o.Width <= 0 {
		return errors.New("image: width must be positive")
	}
	if o.Height != nil && *o.Height <= 0 {
		return errors.New("image: height must be positive")
	}
	if o.OutputFormat != "" {
		if _, err := normalizeOutputFormat(o.OutputFormat); err != nil {
			return err
		}
	}
	if o.ResponseFormat != "" && !o.ResponseFormat.Valid() {
		return fmt.Errorf("image: invalid response format %q", o.ResponseFormat)
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("image: options extra: %w", err)
	}
	return nil
}

// Request is one image-generation call: the prompt and explicit options.
type Request struct {
	// Prompt is the natural-language description of the desired image.
	Prompt string `json:"prompt"`

	Options *Options `json:"options,omitempty"`
}

// NewRequest builds a Request from prompt. Returns an error when prompt
// is empty.
func NewRequest(prompt string) (*Request, error) {
	r := &Request{Prompt: prompt}
	if err := r.Validate(); err != nil {
		return nil, fmt.Errorf("image.NewRequest: %w", err)
	}
	return r, nil
}

// Validate checks the complete request before it crosses a model boundary.
func (r *Request) Validate() error {
	if r == nil {
		return errors.New("image: nil request")
	}
	if r.Prompt == "" {
		return errors.New("image: prompt must not be empty")
	}
	return r.Options.validate()
}
