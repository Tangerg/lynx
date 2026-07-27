package interaction

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

var (
	ErrInvalidEvent = errors.New("interaction: invalid event")
	ErrInvalidID    = errors.New("interaction: invalid ID")
)

// ValidateID verifies a stable suspension or resume identity.
func ValidateID(id string) error {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(id) != id {
		return fmt.Errorf("%w: must be non-empty without surrounding whitespace", ErrInvalidID)
	}
	return nil
}

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

// Resume is the structured input attached to a continued suspension.
type Resume struct {
	ID    string
	Input json.RawMessage
}

// Validate checks the continuation identity and input payload.
func (r Resume) Validate() error {
	if err := ValidateID(r.ID); err != nil {
		return fmt.Errorf("%w: resume ID: %w", ErrInvalidEvent, err)
	}
	if !json.Valid(r.Input) {
		return fmt.Errorf("%w: resume input must be valid JSON", ErrInvalidEvent)
	}
	return nil
}

// Event is the framework-level model/tool boundary. Runtime publishes every
// value with process and deployment ownership; drivers may have richer private
// checkpoint events, but project them onto this shared in-memory shape.
type Event struct {
	Kind       EventKind
	Round      int
	Final      bool
	Cost       float64
	Request    *chat.Request
	Response   *chat.Response
	ToolCall   *chat.ToolCall
	ToolResult *chat.ToolResult
	Suspension *Suspension
	Resume     *Resume
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
	if math.IsNaN(e.Cost) || math.IsInf(e.Cost, 0) || e.Cost < 0 {
		return fmt.Errorf("%w: cost must be finite and non-negative", ErrInvalidEvent)
	}
	if e.Cost != 0 && e.Kind != EventModelResponse {
		return fmt.Errorf("%w: only model responses may carry cost", ErrInvalidEvent)
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
		if e.Resume == nil {
			return e.wrongPayload()
		}
		if err := e.Resume.Validate(); err != nil {
			return fmt.Errorf("%w: resume: %w", ErrInvalidEvent, err)
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

type ToolResolver interface {
	Resolve(name string) (tools.Tool, bool)
}

type Observer func(context.Context, Event) error

// Limits are checked before each continuation model call. Zero leaves a
// dimension unbounded; negative values are invalid.
type Limits struct {
	MaxRounds int
	// MaxConcurrentToolCalls caps conflict-free tool calls executing at once
	// in one model round. Zero selects the tool-loop default. Exclusive tools
	// and calls sharing a non-empty resource key still serialize.
	MaxConcurrentToolCalls int
	// MaxSteps caps model rounds in this one managed interaction.
	MaxSteps int
	// MaxModelCalls caps cumulative model calls already recorded by this
	// process and its descendants. Hosts use it when one application budget
	// must cover a complete delegation tree while MaxSteps retains its local
	// interaction semantics.
	MaxModelCalls int
	MaxTokens     int64
	MaxCost       float64
}

// ErrInvalidLimits identifies malformed managed-interaction limits.
var ErrInvalidLimits = errors.New("interaction limits: invalid")

// Validate checks that every configured limit is finite and non-negative.
// Zero leaves the dimension unbounded, except MaxConcurrentToolCalls where it
// selects the tool-loop default.
func (l Limits) Validate() error {
	if l.MaxRounds < 0 || l.MaxConcurrentToolCalls < 0 || l.MaxSteps < 0 ||
		l.MaxModelCalls < 0 || l.MaxTokens < 0 {
		return fmt.Errorf("%w: integer limits must not be negative", ErrInvalidLimits)
	}
	if math.IsNaN(l.MaxCost) || math.IsInf(l.MaxCost, 0) || l.MaxCost < 0 {
		return fmt.Errorf("%w: cost limit must be finite and non-negative", ErrInvalidLimits)
	}
	return nil
}

type StopReason string

const (
	StopNone   StopReason = ""
	StopBudget StopReason = "budget"
	StopSteps  StopReason = "steps"
)

// Valid reports whether r is a framework-defined interaction stop reason.
func (r StopReason) Valid() bool {
	switch r {
	case StopNone, StopBudget, StopSteps:
		return true
	default:
		return false
	}
}

// Result preserves the complete terminal boundary. Convenience helpers may
// project it to text, but the managed runtime never compresses it internally.
type Result struct {
	Final      *Event
	StopReason StopReason
}
