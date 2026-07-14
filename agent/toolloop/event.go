package toolloop

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
)

// ErrInvalidEvent reports an unknown event kind or an invalid tagged payload.
var ErrInvalidEvent = errors.New("toolloop: invalid event")

// EventKind identifies one boundary crossed by a tool-loop invocation.
type EventKind string

const (
	// EventModelRequest records a provider request boundary.
	EventModelRequest EventKind = "model_request"
	// EventModelResponse records provider output without adding runtime fields
	// to chat.Response.
	EventModelResponse EventKind = "model_response"
	// EventToolCall records a model-requested tool execution.
	EventToolCall EventKind = "tool_call"
	// EventToolResult records one completed tool execution.
	EventToolResult EventKind = "tool_result"
	// EventPause records a resumable control-flow boundary.
	EventPause EventKind = "pause"
	// EventResume records continuation from a matching pause.
	EventResume EventKind = "resume"
)

// Valid reports whether k is a known event kind.
func (k EventKind) Valid() bool {
	switch k {
	case EventModelRequest, EventModelResponse, EventToolCall, EventToolResult, EventPause, EventResume:
		return true
	default:
		return false
	}
}

// Pause identifies a resumable checkpoint and explains why execution stopped.
type Pause struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// Validate verifies checkpoint identity and diagnostic context.
func (p Pause) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("%w: pause ID must not be empty", ErrInvalidEvent)
	}
	if strings.TrimSpace(p.Reason) == "" {
		return fmt.Errorf("%w: pause reason must not be empty", ErrInvalidEvent)
	}
	return nil
}

// Resume identifies the checkpoint being continued. Input carries optional
// operator data; an approval-only resume may leave it empty.
type Resume struct {
	ID    string `json:"id"`
	Input string `json:"input,omitempty"`
}

// Validate verifies checkpoint identity.
func (r Resume) Validate() error {
	if strings.TrimSpace(r.ID) == "" {
		return fmt.Errorf("%w: resume ID must not be empty", ErrInvalidEvent)
	}
	return nil
}

// Event is a serializable tagged value for model, tool, pause, and resume
// boundaries. Kind selects exactly one payload field.
type Event struct {
	Kind       EventKind        `json:"kind"`
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
	if e.payloadCount() != 1 {
		return fmt.Errorf("%w: kind %q requires exactly one payload", ErrInvalidEvent, e.Kind)
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
	case EventResume:
		if e.Resume == nil {
			return e.wrongPayload()
		}
		if err := e.Resume.Validate(); err != nil {
			return err
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
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidEvent, err)
	}
	candidate := Event(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*e = candidate
	return nil
}
