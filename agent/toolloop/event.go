package toolloop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/core/chat"
)

// ErrInvalidEvent reports an unknown event kind or an invalid tagged payload.
var ErrInvalidEvent = errors.New("toolloop: invalid event")

// EventKind identifies one boundary crossed by a tool-loop invocation. The
// interaction package owns the shared model/tool boundary vocabulary.
type EventKind = interaction.EventKind

const (
	// EventModelRequest records a provider request boundary.
	EventModelRequest = interaction.EventModelRequest
	// EventModelResponse records provider output without adding runtime fields
	// to chat.Response.
	EventModelResponse = interaction.EventModelResponse
	// EventToolCall records a model-requested tool execution.
	EventToolCall = interaction.EventToolCall
	// EventToolResult records one completed tool execution.
	EventToolResult = interaction.EventToolResult
	// EventPause records a resumable control-flow boundary.
	EventPause = interaction.EventPause
	// EventResume records continuation from a matching pause.
	EventResume = interaction.EventResume
)

// Pause identifies a resumable checkpoint and explains why execution stopped.
type Pause struct {
	ID           string          `json:"id"`
	Reason       string          `json:"reason"`
	Prompt       json.RawMessage `json:"prompt"`
	ResumeSchema json.RawMessage `json:"resume_schema"`
	Checkpoint   *Checkpoint     `json:"checkpoint"`
}

// Validate verifies checkpoint identity and diagnostic context.
func (p Pause) Validate() error {
	if err := interaction.ValidateID(p.ID); err != nil {
		return fmt.Errorf("%w: pause ID: %w", ErrInvalidEvent, err)
	}
	if strings.TrimSpace(p.Reason) == "" {
		return fmt.Errorf("%w: pause reason must not be empty", ErrInvalidEvent)
	}
	if !json.Valid(p.Prompt) || !json.Valid(p.ResumeSchema) {
		return fmt.Errorf("%w: pause prompt and resume schema must be valid JSON", ErrInvalidEvent)
	}
	if p.Checkpoint == nil {
		return fmt.Errorf("%w: pause checkpoint must not be nil", ErrInvalidEvent)
	}
	if err := p.Checkpoint.Validate(); err != nil {
		return fmt.Errorf("%w: pause checkpoint: %w", ErrInvalidEvent, err)
	}
	if p.Checkpoint.ID != p.ID {
		return fmt.Errorf("%w: pause ID %q does not match checkpoint ID %q", ErrInvalidEvent, p.ID, p.Checkpoint.ID)
	}
	return nil
}

// Resume is the shared interaction continuation payload.
type Resume = interaction.Resume

// Event is a serializable tagged value for model, tool, pause, and resume
// boundaries. Kind selects exactly one payload field.
type Event struct {
	Kind       EventKind        `json:"kind"`
	Round      int              `json:"round"`
	Final      bool             `json:"final,omitempty"`
	Request    *chat.Request    `json:"request,omitempty"`
	Response   *chat.Response   `json:"response,omitempty"`
	ToolCall   *chat.ToolCall   `json:"tool_call,omitempty"`
	ToolResult *chat.ToolResult `json:"tool_result,omitempty"`
	Pause      *Pause           `json:"pause,omitempty"`
	Resume     *Resume          `json:"resume,omitempty"`
}

// Validate verifies the discriminator, payload exclusivity, and active nested
// value.
func (e Event) Validate() error {
	if !e.Kind.Valid() {
		return fmt.Errorf("%w: unknown kind %q", ErrInvalidEvent, e.Kind)
	}
	if e.Round < 1 {
		return fmt.Errorf("%w: round must be positive", ErrInvalidEvent)
	}
	if e.payloadCount() != 1 {
		return fmt.Errorf("%w: kind %q requires exactly one payload", ErrInvalidEvent, e.Kind)
	}
	if e.Final && e.Kind != EventModelResponse && e.Kind != EventToolResult {
		return fmt.Errorf("%w: only model responses and tool results may be final", ErrInvalidEvent)
	}

	switch e.Kind {
	case EventModelRequest:
		if e.Request == nil {
			return e.wrongPayload()
		}
		if err := e.Request.Validate(); err != nil {
			return fmt.Errorf("%w: request: %w", ErrInvalidEvent, err)
		}
	case EventModelResponse:
		if e.Response == nil {
			return e.wrongPayload()
		}
		if err := e.Response.Validate(); err != nil {
			return fmt.Errorf("%w: response: %w", ErrInvalidEvent, err)
		}
	case EventToolCall:
		if e.ToolCall == nil {
			return e.wrongPayload()
		}
		if err := e.ToolCall.Validate(); err != nil {
			return fmt.Errorf("%w: tool call: %w", ErrInvalidEvent, err)
		}
	case EventToolResult:
		if e.ToolResult == nil {
			return e.wrongPayload()
		}
		if err := e.ToolResult.Validate(); err != nil {
			return fmt.Errorf("%w: tool result: %w", ErrInvalidEvent, err)
		}
	case EventPause:
		if e.Pause == nil {
			return e.wrongPayload()
		}
		if err := e.Pause.Validate(); err != nil {
			return err
		}
		if e.Pause.Checkpoint.Round != e.Round {
			return fmt.Errorf("%w: pause round does not match checkpoint", ErrInvalidEvent)
		}
	case EventResume:
		if e.Resume == nil {
			return e.wrongPayload()
		}
		if err := e.Resume.Validate(); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidEvent, err)
		}
	}
	return nil
}

func (e Event) payloadCount() int {
	count := 0
	for _, present := range []bool{
		e.Request != nil,
		e.Response != nil,
		e.ToolCall != nil,
		e.ToolResult != nil,
		e.Pause != nil,
		e.Resume != nil,
	} {
		if present {
			count++
		}
	}
	return count
}

func (e Event) wrongPayload() error {
	return fmt.Errorf("%w: payload does not match kind %q", ErrInvalidEvent, e.Kind)
}

// MarshalJSON validates Event before writing its tagged representation.
func (e Event) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	type wireEvent Event
	return json.Marshal(wireEvent(e))
}

// UnmarshalJSON decodes and validates an Event before replacing the receiver.
func (e *Event) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidEvent)
	}
	type wireEvent Event
	var decoded wireEvent
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidEvent, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON value", ErrInvalidEvent)
	}
	candidate := Event(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*e = candidate
	return nil
}
