package image

import (
	"errors"
	"fmt"
	"mime"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/core/media"
	"github.com/Tangerg/lynx/core/metadata"
)

// ResultMetadata holds per-image metadata returned by the provider.
type ResultMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific result metadata into Extra.
func (m *ResultMetadata) Set(key string, value any) error {
	if m == nil {
		return errors.New("image.ResultMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
}

// Result is one generated image plus its metadata.
type Result struct {
	// Media holds the generated image as bytes or an absolute URI.
	Media *media.Media `json:"media,omitempty"`

	// Metadata carries per-image extras.
	Metadata *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds a [Result]. Returns an error when media or metadata
// is nil.
func NewResult(value *media.Media, metadata *ResultMetadata) (*Result, error) {
	result := &Result{Media: value, Metadata: metadata}
	if err := validateResult(result); err != nil {
		return nil, fmt.Errorf("image.NewResult: %w", err)
	}
	return result, nil
}

// ResponseMetadata holds response-level metadata for an image
// generation request.
type ResponseMetadata struct {
	// Created is the provider-reported creation time, Unix seconds.
	Created int64 `json:"created"`

	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific response metadata into Extra.
func (m *ResponseMetadata) Set(key string, value any) error {
	if m == nil {
		return errors.New("image.ResponseMetadata.Set: nil receiver")
	}
	return m.Extra.Set(key, value)
}

// Response is the full image-generation result: every rendered image plus
// shared response metadata.
type Response struct {
	// Results contains every image returned by the provider, in provider order.
	Results []*Result `json:"results,omitzero"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from at least one result and non-nil
// metadata.
func NewResponse(results []*Result, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Results: slices.Clone(results), Metadata: metadata}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("image.NewResponse: %w", err)
	}
	return response, nil
}

// Validate recursively verifies generated media and response metadata.
func (r *Response) Validate() error {
	if r == nil {
		return errors.New("image.Response: nil response")
	}
	if len(r.Results) == 0 {
		return errors.New("image.Response: at least one result is required")
	}
	for i, result := range r.Results {
		if err := validateResult(result); err != nil {
			return fmt.Errorf("image.Response: results[%d]: %w", i, err)
		}
	}
	if r.Metadata == nil {
		return errors.New("image.Response: metadata must not be nil")
	}
	if r.Metadata.Created < 0 {
		return errors.New("image.Response: created must not be negative")
	}
	if err := r.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("image.Response: metadata: %w", err)
	}
	return nil
}

func validateResult(result *Result) error {
	if result == nil {
		return errors.New("result must not be nil")
	}
	if err := result.Media.Validate(); err != nil {
		return fmt.Errorf("media: %w", err)
	}
	mediaType, _, _ := mime.ParseMediaType(result.Media.MIME)
	if !strings.HasPrefix(mediaType, "image/") && mediaType != "application/octet-stream" {
		return fmt.Errorf("media MIME type %q is not an image", result.Media.MIME)
	}
	if result.Metadata == nil {
		return errors.New("metadata must not be nil")
	}
	if err := result.Metadata.Extra.Validate(); err != nil {
		return fmt.Errorf("metadata: %w", err)
	}
	return nil
}

// First returns the first generated image, or nil when the response is empty.
func (r *Response) First() *Result {
	if r == nil || len(r.Results) == 0 {
		return nil
	}
	return r.Results[0]
}
