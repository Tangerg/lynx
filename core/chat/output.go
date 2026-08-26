package chat

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/metadata"
)

// FinishReason explains why generation stopped. The empty value means that a
// streaming output has not finished yet.
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
// finish reason. Provider-native reasons map to Other and output metadata.
func (r FinishReason) Valid() bool {
	switch r {
	case "", FinishReasonStop, FinishReasonLength, FinishReasonToolCalls, FinishReasonContentFilter, FinishReasonOther:
		return true
	default:
		return false
	}
}

// OutputMetadata holds provider-specific metadata for one generation output.
type OutputMetadata struct {
	Extra metadata.Map `json:"extra,omitzero"`
}

// Set encodes provider-specific output metadata into Extra.
func (m *OutputMetadata) Set(key string, value any) error {
	if m == nil {
		return fmt.Errorf("chat.OutputMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := m.Extra.Set(key, value); err != nil {
		return fmt.Errorf("chat.OutputMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m *OutputMetadata) validate() error {
	if m == nil {
		return nil
	}
	if err := m.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: output metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (m OutputMetadata) clone() *OutputMetadata {
	return &OutputMetadata{Extra: m.Extra.Clone()}
}

// MarshalJSON validates OutputMetadata before writing its wire representation.
func (m OutputMetadata) MarshalJSON() ([]byte, error) {
	if err := (&m).validate(); err != nil {
		return nil, err
	}
	type wireOutputMetadata OutputMetadata
	return json.Marshal(wireOutputMetadata(m))
}

// UnmarshalJSON decodes and validates OutputMetadata before replacing the receiver.
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

// Output is the single provider generation produced by a chat call. Message
// may be nil on a streaming chunk that only carries a finish reason or output
// metadata.
type Output struct {
	Message      *Message        `json:"message,omitempty"`
	FinishReason FinishReason    `json:"finish_reason,omitempty"`
	Metadata     *OutputMetadata `json:"metadata,omitempty"`
}

// NewOutput builds and validates a chat output.
func NewOutput(message *Message, finishReason FinishReason, metadata *OutputMetadata) (*Output, error) {
	output := &Output{Message: message, FinishReason: finishReason, Metadata: metadata}
	if err := output.Validate(); err != nil {
		return nil, fmt.Errorf("chat.NewOutput: %w", err)
	}
	return output, nil
}

// Text returns the assistant text, or an empty string when absent.
func (r *Output) Text() string {
	if r == nil || r.Message == nil {
		return ""
	}
	return r.Message.Text()
}

// Validate verifies the assistant message, normalized finish reason, and
// JSON-safe output metadata.
func (r *Output) Validate() error {
	if r == nil {
		return fmt.Errorf("%w: output must not be nil", ErrInvalidResponse)
	}
	if r.Message == nil && r.FinishReason == "" && r.Metadata == nil {
		return fmt.Errorf("%w: output has no message, finish reason, or metadata", ErrInvalidResponse)
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

func (r Output) clone() *Output {
	clone := r
	if r.Message != nil {
		clone.Message = new(r.Message.Clone())
	}
	if r.Metadata != nil {
		clone.Metadata = r.Metadata.clone()
	}
	return &clone
}

// MarshalJSON validates Output before writing its wire representation.
func (r Output) MarshalJSON() ([]byte, error) {
	if err := (&r).Validate(); err != nil {
		return nil, err
	}
	type wireOutput Output
	return json.Marshal(wireOutput(r))
}

// UnmarshalJSON decodes and validates Output before replacing the receiver.
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
