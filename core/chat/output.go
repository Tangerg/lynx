package chat

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/scope/core/metadata"
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

func (f FinishReason) String() string { return string(f) }

// Valid reports whether f is empty (not finished) or a known normalized
// finish reason. Provider-native reasons map to Other and output metadata.
func (f FinishReason) Valid() bool {
	switch f {
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
func (o *OutputMetadata) Set(key string, value any) error {
	if o == nil {
		return fmt.Errorf("chat.OutputMetadata.Set: %w: nil receiver", ErrInvalidResponse)
	}
	if err := o.Extra.Set(key, value); err != nil {
		return fmt.Errorf("chat.OutputMetadata.Set: %w: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (o *OutputMetadata) validate() error {
	if o == nil {
		return nil
	}
	if err := o.Extra.Validate(); err != nil {
		return fmt.Errorf("%w: output metadata: %w", ErrInvalidResponse, err)
	}
	return nil
}

func (o OutputMetadata) clone() *OutputMetadata {
	return &OutputMetadata{Extra: o.Extra.Clone()}
}

// MarshalJSON validates OutputMetadata before writing its wire representation.
func (o OutputMetadata) MarshalJSON() ([]byte, error) {
	if err := (&o).validate(); err != nil {
		return nil, err
	}
	type wireOutputMetadata OutputMetadata
	return json.Marshal(wireOutputMetadata(o))
}

// UnmarshalJSON decodes and validates OutputMetadata before replacing the receiver.
func (o *OutputMetadata) UnmarshalJSON(data []byte) error {
	if o == nil {
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
	*o = candidate
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
func (o *Output) Text() string {
	if o == nil || o.Message == nil {
		return ""
	}
	return o.Message.Text()
}

// Validate verifies the assistant message, normalized finish reason, and
// JSON-safe output metadata.
func (o *Output) Validate() error {
	if o == nil {
		return fmt.Errorf("%w: output must not be nil", ErrInvalidResponse)
	}
	if o.Message == nil && o.FinishReason == "" && o.Metadata == nil {
		return fmt.Errorf("%w: output has no message, finish reason, or metadata", ErrInvalidResponse)
	}
	if o.Message != nil {
		if err := o.Message.Validate(); err != nil {
			return fmt.Errorf("%w: message: %w", ErrInvalidResponse, err)
		}
		if o.Message.Role != RoleAssistant {
			return fmt.Errorf("%w: message role must be %q, got %q", ErrInvalidResponse, RoleAssistant, o.Message.Role)
		}
	}
	if !o.FinishReason.Valid() {
		return fmt.Errorf("%w: unknown finish reason %q", ErrInvalidResponse, o.FinishReason)
	}
	if err := o.Metadata.validate(); err != nil {
		return err
	}
	return nil
}

func (o Output) clone() *Output {
	clone := o
	if o.Message != nil {
		clone.Message = new(o.Message.Clone())
	}
	if o.Metadata != nil {
		clone.Metadata = o.Metadata.clone()
	}
	return &clone
}

// MarshalJSON validates Output before writing its wire representation.
func (o Output) MarshalJSON() ([]byte, error) {
	if err := (&o).Validate(); err != nil {
		return nil, err
	}
	type wireOutput Output
	return json.Marshal(wireOutput(o))
}

// UnmarshalJSON decodes and validates Output before replacing the receiver.
func (o *Output) UnmarshalJSON(data []byte) error {
	if o == nil {
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
	*o = candidate
	return nil
}
