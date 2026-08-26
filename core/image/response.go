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

// OutputMetadata holds per-image metadata returned by the provider.
type OutputMetadata struct {
	// Extra carries JSON-safe provider-specific metadata.
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific output metadata into Extra.
func (m *OutputMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("image.OutputMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("image.OutputMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *OutputMetadata) validate() error {
	if m == nil {
		return fmt.Errorf("%w: output metadata must not be nil", ErrInvalidResponse)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: output metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m OutputMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireOutputMetadata OutputMetadata
	return json.Marshal(wireOutputMetadata(m))
}

func (m *OutputMetadata) UnmarshalJSON(data []byte) error {
	if m == nil {
		return fmt.Errorf("%w: nil OutputMetadata receiver", ErrInvalidResponse)
	}
	type wireOutputMetadata OutputMetadata
	var decoded wireOutputMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode output metadata: %w", ErrInvalidResponse, err)
	}
	candidate := OutputMetadata(decoded)
	if err := candidate.validate(); err != nil {
		return err
	}
	*m = candidate
	return nil
}

// Output is one generated image plus its metadata.
type Output struct {
	// Media holds the generated image as bytes or an absolute URI.
	Media *media.Media `json:"media,omitempty"`

	// Metadata carries per-image extras.
	Metadata *OutputMetadata `json:"metadata,omitempty"`
}

// NewOutput builds a [Output]. Returns an error when media or metadata
// is nil.
func NewOutput(value *media.Media, metadata *OutputMetadata) (*Output, error) {
	output := &Output{Media: value, Metadata: metadata}
	if err := output.Validate(); err != nil {
		return nil, fmt.Errorf("image.NewOutput: %w", err)
	}
	return output, nil
}

// Validate verifies generated media and output metadata.
func (r *Output) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: output must not be nil", ErrInvalidResponse)
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

func (r Output) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireOutput Output
	return json.Marshal(wireOutput(r))
}

func (r *Output) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("%w: nil Output receiver", ErrInvalidResponse)
	}
	type wireOutput Output
	var decoded wireOutput
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode output: %w", ErrInvalidResponse, err)
	}
	candidate := Output(decoded)
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

// Response is the full image-generation output: every rendered image plus
// shared response metadata.
type Response struct {
	// Outputs contains every image returned by the provider, in provider order.
	Outputs []*Output `json:"outputs,omitzero"`

	// Metadata carries shared response-level fields.
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds a [Response] from at least one output and non-nil
// metadata.
func NewResponse(outputs []*Output, metadata *ResponseMetadata) (*Response, error) {
	response := &Response{Outputs: slices.Clone(outputs), Metadata: metadata}
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
	if len(r.Outputs) == 0 {
		return fmt.Errorf("%w: at least one output is required", ErrInvalidResponse)
	}
	for i, output := range r.Outputs {
		if err := output.Validate(); err != nil {
			return fmt.Errorf("%w: outputs[%d]: %w", ErrInvalidResponse, i, err)
		}
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

// First returns the first generated image, or nil when the response is empty.
func (r *Response) First() *Output {
	if r == nil || len(r.Outputs) == 0 {
		return nil
	}
	return r.Outputs[0]
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
