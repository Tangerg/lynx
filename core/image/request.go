package image

import (
	"fmt"
	"mime"
	"strings"

	"github.com/Tangerg/lynx/core/internal/ptr"
	"github.com/Tangerg/lynx/core/metadata"
)

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

	// Seed pins the RNG so repeated calls produce the same image.
	Seed *int64 `json:"seed,omitempty"`

	// OutputFormat picks the image MIME type of the rendered bytes.
	// Empty leaves the format to the provider.
	OutputFormat string `json:"output_format,omitempty"`

	// Extra carries JSON-safe provider-specific options unknown to this struct.
	Extra metadata.Map `json:"extra,omitzero"`
}

// NewOptions builds Options for the given model id. Returns an error
// when model is empty or has surrounding whitespace.
func NewOptions(model string) (*Options, error) {
	if model == "" {
		return nil, fmt.Errorf("image.NewOptions: %w: model id must not be empty", ErrInvalidOptions)
	}
	if strings.TrimSpace(model) != model {
		return nil, fmt.Errorf("image.NewOptions: %w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	return &Options{Model: model}, nil
}

// Set encodes a provider-specific option into Extra.
func (o *Options) Set(key string, value any) error {
	if o == nil {
		return fmt.Errorf("image.Options.Set: %w: nil receiver", ErrInvalidOptions)
	}
	if err := o.Extra.Set(key, value); err != nil {
		return fmt.Errorf("image.Options.Set: %w: %w", ErrInvalidOptions, err)
	}
	return nil
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
		Seed:           ptr.Clone(o.Seed),
		OutputFormat:   o.OutputFormat,
		Extra:          o.Extra.Clone(),
	}
}

// Merged clones o then applies each override left-to-right.
// Scalar non-zero values overwrite; the Extra map merges last-write-wins.
// A nil receiver returns an error.
func (o *Options) Merged(overrides ...*Options) (*Options, error) {
	if o == nil {
		return nil, fmt.Errorf("image.Options.Merged: %w: nil receiver", ErrInvalidOptions)
	}

	merged := o.Clone()
	for _, override := range overrides {
		if override == nil {
			continue
		}
		if err := merged.applyOverride(override); err != nil {
			return nil, fmt.Errorf("image.Options.Merged: %w: %w", ErrInvalidOptions, err)
		}
	}
	normalized, err := normalizeOutputFormat(merged.OutputFormat)
	if err != nil {
		return nil, fmt.Errorf("image.Options.Merged: %w: %w", ErrInvalidOptions, err)
	}
	merged.OutputFormat = normalized
	if err := merged.validate(); err != nil {
		return nil, fmt.Errorf("image.Options.Merged: %w", err)
	}
	return merged, nil
}

func normalizeOutputFormat(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	mediaType, parameters, err := mime.ParseMediaType(value)
	if err != nil {
		return "", fmt.Errorf("invalid output format %q: %w", value, err)
	}
	mediaType = strings.ToLower(mediaType)
	if !strings.HasPrefix(mediaType, "image/") || len(strings.TrimPrefix(mediaType, "image/")) == 0 {
		return "", fmt.Errorf("output format %q is not an image MIME type", value)
	}
	if len(parameters) != 0 {
		return "", fmt.Errorf("output format %q must not include parameters", value)
	}
	return mediaType, nil
}

func (o *Options) applyOverride(src *Options) error {
	if src.NegativePrompt != "" {
		o.NegativePrompt = src.NegativePrompt
	}
	if src.Model != "" {
		o.Model = src.Model
	}
	if src.Width != nil {
		o.Width = ptr.Clone(src.Width)
	}
	if src.Height != nil {
		o.Height = ptr.Clone(src.Height)
	}
	if src.Seed != nil {
		o.Seed = ptr.Clone(src.Seed)
	}
	if src.OutputFormat != "" {
		o.OutputFormat = src.OutputFormat
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
	if o.Model != "" && strings.TrimSpace(o.Model) != o.Model {
		return fmt.Errorf("%w: model id must not have surrounding whitespace", ErrInvalidOptions)
	}
	if o.Width != nil && *o.Width <= 0 {
		return fmt.Errorf("%w: width must be positive", ErrInvalidOptions)
	}
	if o.Height != nil && *o.Height <= 0 {
		return fmt.Errorf("%w: height must be positive", ErrInvalidOptions)
	}
	if o.Seed != nil && *o.Seed < 0 {
		return fmt.Errorf("%w: seed must not be negative", ErrInvalidOptions)
	}
	if o.OutputFormat != "" {
		normalized, err := normalizeOutputFormat(o.OutputFormat)
		if err != nil {
			return fmt.Errorf("%w: output format: %w", ErrInvalidOptions, err)
		}
		if normalized != o.OutputFormat {
			return fmt.Errorf("%w: output format must use canonical MIME form %q", ErrInvalidOptions, normalized)
		}
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: extra: %w", ErrInvalidOptions, err)
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
		return fmt.Errorf("%w: nil request", ErrInvalidRequest)
	}
	if r.Prompt == "" {
		return fmt.Errorf("%w: prompt must not be empty", ErrInvalidRequest)
	}
	if err := r.Options.validate(); err != nil {
		return fmt.Errorf("%w: options: %w", ErrInvalidRequest, err)
	}
	return nil
}
