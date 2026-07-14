package toolloop

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
)

var (
	// ErrInvalidCheckpoint reports malformed or internally inconsistent pause
	// state.
	ErrInvalidCheckpoint = errors.New("toolloop: invalid checkpoint")
	// ErrAmbiguousToolCalls reports a response whose multiple choices would
	// require the runtime to guess which tool-call branch to execute.
	ErrAmbiguousToolCalls = errors.New("toolloop: ambiguous tool calls")
)

// Checkpoint is the complete serializable state needed to continue a paused
// tool round without invoking the model again or re-running completed calls.
// It deliberately contains no executable ToolResolver; Resume receives that
// capability separately.
type Checkpoint struct {
	ID       string            `json:"id"`
	Round    int               `json:"round"`
	Request  *chat.Request     `json:"request"`
	Response *chat.Response    `json:"response"`
	Results  []chat.ToolResult `json:"results,omitempty"`
	NextCall int               `json:"next_call"`
}

// Validate verifies the protocol snapshots and their call/result correlation.
func (c *Checkpoint) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidCheckpoint)
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: ID must not be empty", ErrInvalidCheckpoint)
	}
	if c.Round < 1 {
		return fmt.Errorf("%w: round must be positive", ErrInvalidCheckpoint)
	}
	if c.Request == nil {
		return fmt.Errorf("%w: request must not be nil", ErrInvalidCheckpoint)
	}
	if err := c.Request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidCheckpoint, err)
	}
	if c.Response == nil {
		return fmt.Errorf("%w: response must not be nil", ErrInvalidCheckpoint)
	}
	calls, err := responseToolCalls(c.Response)
	if err != nil {
		return fmt.Errorf("%w: response: %w", ErrInvalidCheckpoint, err)
	}
	if len(calls) == 0 {
		return fmt.Errorf("%w: response has no tool calls", ErrInvalidCheckpoint)
	}
	if c.NextCall < 0 || c.NextCall >= len(calls) {
		return fmt.Errorf("%w: next call %d is outside [0,%d)", ErrInvalidCheckpoint, c.NextCall, len(calls))
	}
	if len(c.Results) != c.NextCall {
		return fmt.Errorf("%w: %d completed results do not match next call %d", ErrInvalidCheckpoint, len(c.Results), c.NextCall)
	}
	for i := range c.Results {
		if err := c.Results[i].Validate(); err != nil {
			return fmt.Errorf("%w: results[%d]: %w", ErrInvalidCheckpoint, i, err)
		}
		if c.Results[i].ID != calls[i].ID || c.Results[i].Name != calls[i].Name {
			return fmt.Errorf("%w: results[%d] does not match tool call %q", ErrInvalidCheckpoint, i, calls[i].ID)
		}
	}
	return nil
}

// MarshalJSON validates a checkpoint before writing it.
func (c Checkpoint) MarshalJSON() ([]byte, error) {
	if err := (&c).Validate(); err != nil {
		return nil, err
	}
	type wireCheckpoint Checkpoint
	return json.Marshal(wireCheckpoint(c))
}

// UnmarshalJSON decodes and validates a checkpoint before replacing the
// receiver.
func (c *Checkpoint) UnmarshalJSON(data []byte) error {
	if c == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidCheckpoint)
	}
	type wireCheckpoint Checkpoint
	var decoded wireCheckpoint
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidCheckpoint, err)
	}
	candidate := Checkpoint(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*c = candidate
	return nil
}

func responseToolCalls(response *chat.Response) ([]chat.ToolCall, error) {
	if response == nil {
		return nil, fmt.Errorf("%w: nil response", ErrAmbiguousToolCalls)
	}
	if err := response.Validate(); err != nil {
		return nil, err
	}

	var calls []chat.ToolCall
	seen := make(map[string]struct{})
	for choiceIndex := range response.Choices {
		choice := &response.Choices[choiceIndex]
		if choice.Message == nil {
			continue
		}
		for partIndex := range choice.Message.Parts {
			part := &choice.Message.Parts[partIndex]
			if part.Kind != chat.PartToolCall {
				continue
			}
			if choiceIndex != 0 {
				return nil, fmt.Errorf("%w: choice %d contains executable calls; only the first choice is canonical", ErrAmbiguousToolCalls, choice.Index)
			}
			if _, duplicate := seen[part.ToolCall.ID]; duplicate {
				return nil, fmt.Errorf("%w: duplicate call ID %q", ErrAmbiguousToolCalls, part.ToolCall.ID)
			}
			seen[part.ToolCall.ID] = struct{}{}
			calls = append(calls, *part.ToolCall)
		}
	}
	return calls, nil
}
