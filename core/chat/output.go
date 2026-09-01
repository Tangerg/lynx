package chat

import (
	"encoding/json"
	"fmt"

	"github.com/Tangerg/scope/core/metadata"
)

// FinishReason explains why generation stopped. Complete outputs require a
// non-empty value; ResponseDelta uses the empty value before termination.
type FinishReason string

const (
	// FinishReasonStop means the model reached a natural stop condition.
	FinishReasonStop FinishReason = "stop"
	// FinishReasonLength means a configured or provider limit ended generation.
	FinishReasonLength FinishReason = "length"
	// FinishReasonToolCalls means the model stopped to request tool execution.
	FinishReasonToolCalls FinishReason = "tool_calls"
	// FinishReasonContentFilter means provider policy stopped generation.
	FinishReasonContentFilter FinishReason = "content_filter"
	// FinishReasonRefusal means the model declined the request.
	FinishReasonRefusal FinishReason = "refusal"
	// FinishReasonOther preserves a known terminal state with no portable match.
	FinishReasonOther FinishReason = "other"
)

func (f FinishReason) String() string { return string(f) }

func (f FinishReason) Valid() bool {
	switch f {
	case FinishReasonStop, FinishReasonLength, FinishReasonToolCalls, FinishReasonContentFilter, FinishReasonRefusal, FinishReasonOther:
		return true
	default:
		return false
	}
}

// OutputMetadata holds provider-specific metadata for one generation output.
type OutputMetadata struct {
	Extra metadata.Map `json:"extra,omitzero"`
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

func (o OutputMetadata) MarshalJSON() ([]byte, error) {
	if err := (&o).validate(); err != nil {
		return nil, err
	}
	type wireOutputMetadata OutputMetadata
	return json.Marshal(wireOutputMetadata(o))
}

func (o *OutputMetadata) UnmarshalJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("%w: output metadata receiver is nil", ErrInvalidResponse)
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

// Output is the complete single provider generation produced by a chat call.
// Message may be nil when the provider completed without a portable content
// item, but FinishReason is always present.
type Output struct {
	Message      *Message        `json:"message,omitempty"`
	FinishReason FinishReason    `json:"finish_reason,omitempty"`
	Metadata     *OutputMetadata `json:"metadata,omitempty"`
}

// NewOutput validates the single stable generation promoted from a provider
// response or accumulated stream.
func NewOutput(message *Message, finishReason FinishReason, outputMetadata *OutputMetadata) (*Output, error) {
	output := &Output{Message: message, FinishReason: finishReason, Metadata: outputMetadata}
	if err := output.Validate(); err != nil {
		return nil, fmt.Errorf("chat: create output: %w", err)
	}
	return output, nil
}

func (o *Output) Text() string {
	if o == nil || o.Message == nil {
		return ""
	}
	return o.Message.Text()
}

func (o *Output) Validate() error {
	if o == nil {
		return fmt.Errorf("%w: output must not be nil", ErrInvalidResponse)
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
		return fmt.Errorf("%w: missing or unknown finish reason %q", ErrInvalidResponse, o.FinishReason)
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

func (o Output) MarshalJSON() ([]byte, error) {
	if err := (&o).Validate(); err != nil {
		return nil, err
	}
	type wireOutput Output
	return json.Marshal(wireOutput(o))
}

func (o *Output) UnmarshalJSON(data []byte) error {
	if o == nil {
		return fmt.Errorf("%w: output receiver is nil", ErrInvalidResponse)
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
