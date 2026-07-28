package interaction

import (
	"bytes"
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

// EventKind names the boundary an [Event] reports. The set is closed: a driver
// projects its own protocol onto these, so a new kind is a change to what every
// consumer must handle, not an extension point.
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

// Clone returns an independent protocol-value snapshot of e.
func (e Event) Clone() Event {
	cloned := e
	cloned.Request = e.Request.Clone()
	cloned.Response = e.Response.Clone()
	if e.ToolCall != nil {
		toolCall := *e.ToolCall
		cloned.ToolCall = &toolCall
	}
	if e.ToolResult != nil {
		toolResult := *e.ToolResult
		cloned.ToolResult = &toolResult
	}
	if e.Suspension != nil {
		cloned.Suspension = e.Suspension.Clone()
	}
	if e.Resume != nil {
		resume := *e.Resume
		resume.Input = bytes.Clone(e.Resume.Input)
		cloned.Resume = &resume
	}
	return cloned
}

// Validate reports whether e is internally consistent, wrapping
// [ErrInvalidEvent]: each kind requires its own payload, and the shared shape
// cannot express that in types.
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

// ToolResolver resolves the executable tool advertised by a model request.
// Resolve must return a non-nil Tool whenever ok is true. A driver reports a
// resolver panic as an execution error attributed to the requested tool name,
// so an implementation may fail without unwinding the loop.
type ToolResolver interface {
	Resolve(name string) (tools.Tool, bool)
}

// Limits bound one managed interaction. Zero leaves a dimension unbounded and
// negative values are invalid, so the framework never picks a number on the
// host's behalf.
type Limits struct {
	// MaxRounds caps the model rounds of this interaction. Reaching it ends the
	// interaction with StopSteps rather than an error: a bound the host asked
	// for is an expected outcome, not a fault.
	MaxRounds int
	// MaxConcurrentToolCalls caps conflict-free tool calls executing at once
	// in one model round. Zero runs them one at a time. Exclusive tools and
	// calls sharing a non-empty resource key still serialize.
	MaxConcurrentToolCalls int
}

// ErrInvalidLimits identifies malformed managed-interaction limits.
var ErrInvalidLimits = errors.New("interaction limits: invalid")

// Validate checks that every configured limit is non-negative.
func (l Limits) Validate() error {
	if l.MaxRounds < 0 || l.MaxConcurrentToolCalls < 0 {
		return fmt.Errorf("%w: integer limits must not be negative", ErrInvalidLimits)
	}
	return nil
}

// StopReason names the bound that ended an interaction before it reached a
// final model or tool boundary. Every value describes an outcome the host asked
// for by configuring a limit, so none of them is an error.
type StopReason string

const (
	StopNone StopReason = ""
	// StopBudget reports that the tree's cost or token budget is exhausted.
	StopBudget StopReason = "budget"
	// StopSteps reports that a step bound stopped continuation: either the
	// tree's model-call budget or this interaction's Limits.MaxRounds.
	StopSteps StopReason = "steps"
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
