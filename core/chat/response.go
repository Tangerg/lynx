package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/metadata"
)

// ErrInvalidResponse reports malformed provider response data.
var ErrInvalidResponse = errors.New("chat: invalid response")

// ResponseMetadata holds provider identity, usage, and response-scoped extras.
type ResponseMetadata struct {
	ID    string       `json:"id,omitempty"`
	Model string       `json:"model,omitempty"`
	Usage Usage        `json:"usage,omitzero"`
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific response metadata into Extra.
func (r *ResponseMetadata) Set(key string, value any) error {
	if r == nil {
		return fmt.Errorf("chat.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := r.Extra.Set(key, value); err != nil {
		return fmt.Errorf("chat.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r *ResponseMetadata) validate() error {
	if r == nil {
		return nil
	}
	if r.ID != "" && strings.TrimSpace(r.ID) != r.ID {
		return fmt.Errorf("%w: response metadata ID must not have surrounding whitespace", ErrInvalidResponse)
	}
	if r.Model != "" && strings.TrimSpace(r.Model) != r.Model {
		return fmt.Errorf("%w: response metadata model must not have surrounding whitespace", ErrInvalidResponse)
	}
	if err := r.Usage.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	if err := r.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: response metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (r *ResponseMetadata) merge(src ResponseMetadata) error {
	if src.ID != "" {
		r.ID = src.ID
	}
	if src.Model != "" {
		r.Model = src.Model
	}
	if !src.Usage.isZero() {
		r.Usage = src.Usage.clone()
	}
	if err := r.Extra.Merge(src.Extra); err != nil {
		return fmt.Errorf("merge extras: %w", err)
	}
	return nil
}

func (r ResponseMetadata) clone() *ResponseMetadata {
	clone := r
	clone.Usage = r.Usage.clone()
	clone.Extra = r.Extra.Clone()
	return &clone
}

// MarshalJSON validates ResponseMetadata before writing its wire representation.
func (r ResponseMetadata) MarshalJSON() ([]byte, error) {
	if err := (&r).validate(); err != nil {
		return nil, err
	}
	type wireResponseMetadata ResponseMetadata
	return json.Marshal(wireResponseMetadata(r))
}

// UnmarshalJSON decodes and validates ResponseMetadata before replacing the receiver.
func (r *ResponseMetadata) UnmarshalJSON(data []byte) error {
	if r == nil {
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
	*r = candidate
	return nil
}

// Response is provider output with at most one generation output. Its zero
// value is valid so a stream can represent an empty or metadata-only chunk.
type Response struct {
	Output   *Output           `json:"output,omitempty"`
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds and validates a response from one output and shared metadata.
func NewResponse(output *Output, metadata *ResponseMetadata) (*Response, error) {
	if output == nil {
		return nil, fmt.Errorf("chat.NewResponse: %w: output must not be nil", ErrInvalidResponse)
	}
	response := &Response{Output: output, Metadata: metadata}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("chat.NewResponse: %w", err)
	}
	return response, nil
}

// Clone returns an independent copy of r. It is nil-safe.
func (r *Response) Clone() *Response {
	if r == nil {
		return nil
	}
	clone := &Response{}
	if r.Output != nil {
		clone.Output = r.Output.clone()
	}
	if r.Metadata != nil {
		clone.Metadata = r.Metadata.clone()
	}
	return clone
}

// Text returns the output's assistant text. It is nil/empty-safe.
func (r *Response) Text() string {
	if r == nil {
		return ""
	}
	return r.Output.Text()
}

// Validate recursively verifies response data. A nil Output is valid for
// stream chunks that only carry usage or provider metadata.
func (r *Response) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if r.Output != nil {
		if err := r.Output.Validate(); err != nil {
			return fmt.Errorf("%w: output: %w", ErrInvalidResponse, err)
		}
	}
	if err := r.Metadata.validate(); err != nil {
		return fmt.Errorf("%w: metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

// MarshalJSON validates Response before writing its wire representation.
func (r Response) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireResponse Response
	return json.Marshal(wireResponse(r))
}

// UnmarshalJSON decodes and validates Response before replacing the receiver.
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
