package image

import (
	"encoding/json"
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
		return fmt.Errorf("image.ResultMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("image.ResultMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *ResultMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("%w: result metadata must not be nil", ErrInvalidResponse)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: result metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m ResultMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireResultMetadata ResultMetadata
	return json.Marshal(wireResultMetadata(m))
}

func (m *ResultMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("%w: nil ResultMetadata receiver", ErrInvalidResponse)
	}
	type wireResultMetadata ResultMetadata
	var decoded wireResultMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode result metadata: %w", ErrInvalidResponse, err)
	}
	candidate := ResultMetadata(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
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
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("image.NewResult: %w", err)
	}
	return result, nil
}

// Validate verifies generated media and result metadata.
func (r *Result) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: result must not be nil", ErrInvalidResponse)
	}
	if err := r.Media.Validate(); err != nil {
		return fmt.Errorf("%w: media: %w", ErrInvalidResponse, err)
	}
	mediaType, _, _ := mime.ParseMediaType(r.Media.MIME)
	if !strings.HasPrefix(mediaType, "image/") && mediaType != "application/octet-stream" {
		return fmt.Errorf("%w: media MIME type %q is not an image", ErrInvalidResponse, r.Media.MIME)
	}
	if err := r.Metadata.validate(); err != nil {
		return err
	}
	return nil
}

func (r Result) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireResult Result
	return json.Marshal(wireResult(r))
}

func (r *Result) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil Result receiver", ErrInvalidResponse)
	}
	type wireResult Result
	var decoded wireResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode result: %w", ErrInvalidResponse, err)
	}
	candidate := Result(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
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
		return fmt.Errorf("image.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("image.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *ResponseMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("%w: response metadata must not be nil", ErrInvalidResponse)
	}
	if m.Created < 0 {
		return fmt.Errorf("%w: created must not be negative", ErrInvalidResponse)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: response metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m ResponseMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireResponseMetadata ResponseMetadata
	return json.Marshal(wireResponseMetadata(m))
}

func (m *ResponseMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("%w: nil ResponseMetadata receiver", ErrInvalidResponse)
	}
	type wireResponseMetadata ResponseMetadata
	var decoded wireResponseMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode response metadata: %w", ErrInvalidResponse, err)
	}
	candidate := ResponseMetadata(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
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
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if len(r.Results) == 0 {
		return fmt.Errorf("%w: at least one result is required", ErrInvalidResponse)
	}
	for i, result := range r.Results {
		if err := result.Validate(); err != nil {
			return fmt.Errorf("%w: results[%d]: %w", ErrInvalidResponse, i, err)
		}
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
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

func (r Response) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireResponse Response
	return json.Marshal(wireResponse(r))
}

func (r *Response) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil Response receiver", ErrInvalidResponse)
	}
	type wireResponse Response
	var decoded wireResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode response: %w", ErrInvalidResponse, err)
	}
	candidate := Response(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*r = candidate
	return nil
}
