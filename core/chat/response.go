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
func (m *ResponseMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("chat.ResponseMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("chat.ResponseMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *ResponseMetadata) validate() error {
	if m == nil {
		return nil
	}
	if m.ID != "" && strings.TrimSpace(m.ID) != m.ID {
		return fmt.Errorf("%w: response metadata ID must not have surrounding whitespace", ErrInvalidResponse)
	}
	if m.Model != "" && strings.TrimSpace(m.Model) != m.Model {
		return fmt.Errorf("%w: response metadata model must not have surrounding whitespace", ErrInvalidResponse)
	}
	if err := m.Usage.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidResponse, err)
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: response metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *ResponseMetadata) merge(src ResponseMetadata) error {
	if src.ID != "" {
		m.ID = src.ID
	}
	if src.Model != "" {
		m.Model = src.Model
	}
	if !src.Usage.isZero() {
		m.Usage = src.Usage.clone()
	}
	if err := m.Extra.Merge(src.Extra); err != nil {
		return fmt.Errorf("merge extras: %w", err)
	}
	return nil
}

func (m ResponseMetadata) clone() *ResponseMetadata {
	clone := m
	clone.Usage = m.Usage.clone()
	clone.Extra = m.Extra.Clone()
	return &clone
}

// MarshalJSON validates ResponseMetadata before writing its wire representation.
func (m ResponseMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireResponseMetadata ResponseMetadata
	return json.Marshal(wireResponseMetadata(m))
}

// UnmarshalJSON decodes and validates ResponseMetadata before replacing the receiver.
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

// Response is provider output with at most one generation result. Its zero
// value is valid so a stream can represent an empty or metadata-only chunk.
type Response struct {
	Result   *Result           `json:"result,omitempty"`
	Metadata *ResponseMetadata `json:"metadata,omitempty"`
}

// NewResponse builds and validates a response from one result and shared metadata.
func NewResponse(result *Result, metadata *ResponseMetadata) (*Response, error) {
	if result == nil {
		return nil, fmt.Errorf("chat.NewResponse: %w: result must not be nil", ErrInvalidResponse)
	}
	response := &Response{Result: result, Metadata: metadata}
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
	if r.Result != nil {
		clone.Result = r.Result.clone()
	}
	if r.Metadata != nil {
		clone.Metadata = r.Metadata.clone()
	}
	return clone
}

// Text returns the result's assistant text. It is nil/empty-safe.
func (r *Response) Text() string {
	if r == nil {
		return ""
	}
	return r.Result.Text()
}

// Validate recursively verifies response data. A nil Result is valid for
// stream chunks that only carry usage or provider metadata.
func (r *Response) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: nil response", ErrInvalidResponse)
	}
	if r.Result != nil {
		if err := r.Result.Validate(); err != nil {
			return fmt.Errorf("%w: result: %w", ErrInvalidResponse, err)
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
