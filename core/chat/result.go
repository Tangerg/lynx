package chat

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/metadata"
)

// FinishReason explains why generation stopped. The empty value means that a
// streaming result has not finished yet.
type FinishReason string

const (
	FinishReasonStop          FinishReason = "stop"
	FinishReasonLength        FinishReason = "length"
	FinishReasonToolCalls     FinishReason = "tool_calls"
	FinishReasonContentFilter FinishReason = "content_filter"
	FinishReasonOther         FinishReason = "other"
)

func (r FinishReason) String() string { return string(r) }

// Valid reports whether r is empty (not finished) or a known normalized
// finish reason. Provider-native reasons map to Other and result metadata.
func (r FinishReason) Valid() bool {
	switch r {
	case "", FinishReasonStop, FinishReasonLength, FinishReasonToolCalls, FinishReasonContentFilter, FinishReasonOther:
		return true
	default:
		return false
	}
}

// ResultMetadata holds provider-specific metadata for one generation result.
type ResultMetadata struct {
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific result metadata into Extra.
func (m *ResultMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("chat.ResultMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("chat.ResultMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *ResultMetadata) validate() error {
	if m == nil {
		return nil
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: result metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m ResultMetadata) clone() *ResultMetadata {
	return &ResultMetadata{Extra: m.Extra.Clone()}
}

// MarshalJSON validates ResultMetadata before writing its wire representation.
func (m ResultMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireResultMetadata ResultMetadata
	return json.Marshal(wireResultMetadata(m))
}

// UnmarshalJSON decodes and validates ResultMetadata before replacing the receiver.
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

// Result is the single provider generation produced by a chat call. Message
// may be nil on a streaming chunk that only carries a finish reason or result
// metadata.
type Result struct {
	Message      *Message        `json:"message,omitempty"`
	FinishReason FinishReason    `json:"finish_reason,omitempty"`
	Metadata     *ResultMetadata `json:"metadata,omitempty"`
}

// NewResult builds and validates a chat result.
func NewResult(message *Message, finishReason FinishReason, metadata *ResultMetadata) (*Result, error) {
	result := &Result{Message: message, FinishReason: finishReason, Metadata: metadata}
	if err := result.Validate(); err != nil {
		return nil, fmt.Errorf("chat.NewResult: %w", err)
	}
	return result, nil
}

// Text returns the assistant text, or an empty string when absent.
func (r *Result) Text() string {
	if r == nil || r.Message == nil {
		return ""
	}
	return r.Message.Text()
}

// Validate verifies the assistant message, normalized finish reason, and
// JSON-safe result metadata.
func (r *Result) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: result must not be nil", ErrInvalidResponse)
	}
	if r.Message == nil && r.FinishReason == "" && r.Metadata == nil {
		return fmt.Errorf("%w: result has no message, finish reason, or metadata", ErrInvalidResponse)
	}
	if r.Message != nil {
		if err := r.Message.Validate(); err != nil {
			return fmt.Errorf("%w: message: %w", ErrInvalidResponse, err)
		}
		if r.Message.Role != RoleAssistant {
			return fmt.Errorf("%w: message role must be %q, got %q", ErrInvalidResponse, RoleAssistant, r.Message.Role)
		}
	}
	if !r.FinishReason.Valid() {
		return fmt.Errorf("%w: unknown finish reason %q", ErrInvalidResponse, r.FinishReason)
	}
	if err := r.Metadata.validate(); err != nil {
		return err
	}
	return nil
}

func (r Result) clone() *Result {
	clone := r
	if r.Message != nil {
		clone.Message = new(r.Message.Clone())
	}
	if r.Metadata != nil {
		clone.Metadata = r.Metadata.clone()
	}
	return &clone
}

// MarshalJSON validates Result before writing its wire representation.
func (r Result) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireResult Result
	return json.Marshal(wireResult(r))
}

// UnmarshalJSON decodes and validates Result before replacing the receiver.
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
