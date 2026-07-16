package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

var ErrInvalidEvent = errors.New("interaction: invalid event")

type EventKind string

const (
	EventModelRequest  EventKind = "model_request"
	EventModelResponse EventKind = "model_response"
	EventToolCall      EventKind = "tool_call"
	EventToolResult    EventKind = "tool_result"
	EventPause         EventKind = "pause"
	EventResume        EventKind = "resume"
)

func (k EventKind) Valid() bool {
	switch k {
	case EventModelRequest, EventModelResponse, EventToolCall, EventToolResult, EventPause, EventResume:
		return true
	default:
		return false
	}
}

// Resume is the JSON-safe input attached to a continued suspension.
type Resume struct {
	ID    string          `json:"id"`
	Input json.RawMessage `json:"input"`
}

// Event is the framework-level model/tool boundary. Runtime publishes every
// value with process and deployment ownership; drivers may have richer private
// checkpoint events, but must project them onto this stable shape.
type Event struct {
	Kind       EventKind        `json:"kind"`
	Round      int              `json:"round"`
	Final      bool             `json:"final,omitempty"`
	Request    *chat.Request    `json:"request,omitempty"`
	Response   *chat.Response   `json:"response,omitempty"`
	ToolCall   *chat.ToolCall   `json:"tool_call,omitempty"`
	ToolResult *chat.ToolResult `json:"tool_result,omitempty"`
	Suspension *Suspension      `json:"suspension,omitempty"`
	Resume     *Resume          `json:"resume,omitempty"`
}

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
			return fmt.Errorf("%w: model request: %w", ErrInvalidEvent, err)
		}
	case EventModelResponse:
		if e.Response == nil {
			return e.wrongPayload()
		}
		if err := e.Response.Validate(); err != nil {
			return fmt.Errorf("%w: model response: %w", ErrInvalidEvent, err)
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
		if e.Suspension == nil {
			return e.wrongPayload()
		}
		if err := e.Suspension.Validate(); err != nil {
			return fmt.Errorf("%w: suspension: %w", ErrInvalidEvent, err)
		}
	case EventResume:
		if e.Resume == nil || strings.TrimSpace(e.Resume.ID) == "" || strings.TrimSpace(e.Resume.ID) != e.Resume.ID || !json.Valid(e.Resume.Input) {
			return e.wrongPayload()
		}
	}
	return nil
}

func (e Event) payloadCount() int {
	count := 0
	for _, present := range []bool{e.Request != nil, e.Response != nil, e.ToolCall != nil, e.ToolResult != nil, e.Suspension != nil, e.Resume != nil} {
		if present {
			count++
		}
	}
	return count
}

func (e Event) wrongPayload() error {
	return fmt.Errorf("%w: payload does not match kind %q", ErrInvalidEvent, e.Kind)
}

func (e Event) MarshalJSON() ([]byte, error) {
	if err := e.Validate(); err != nil {
		return nil, err
	}
	type wire Event
	return json.Marshal(wire(e))
}

func (e *Event) UnmarshalJSON(data []byte) error {
	if e == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidEvent)
	}
	type wire Event
	var decoded wire
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

type ToolResolver interface {
	Resolve(name string) (tools.Tool, bool)
}

type Observer func(context.Context, Event) error

// Limits are checked before each continuation model call. Zero leaves a
// dimension unbounded; negative values are invalid.
type Limits struct {
	MaxRounds  int
	MaxSteps   int
	MaxTokens  int64
	MaxCostUSD float64
}

type StopReason string

const (
	StopNone   StopReason = ""
	StopBudget StopReason = "budget"
	StopSteps  StopReason = "steps"
)

// Result preserves the complete terminal boundary. Convenience helpers may
// project it to text, but the managed runtime never compresses it internally.
type Result struct {
	Final      *Event
	StopReason StopReason
}
