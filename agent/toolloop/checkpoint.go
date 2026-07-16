package toolloop

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// CheckpointSchemaVersion is the durable tool-loop checkpoint schema accepted
// by this version of the runner. The framework intentionally accepts exactly
// one schema: callers must not guess how to resume obsolete execution state.
const CheckpointSchemaVersion uint16 = 2

// CallStatus identifies the durable state of one model-requested tool call.
// Running is deliberately absent: Runner reaches a checkpoint only after every
// call started in the current concurrency segment has settled.
type CallStatus string

const (
	CallQueued    CallStatus = "queued"
	CallCompleted CallStatus = "completed"
	CallPaused    CallStatus = "paused"
)

// Valid reports whether s is a framework-defined call status.
func (s CallStatus) Valid() bool {
	switch s {
	case CallQueued, CallCompleted, CallPaused:
		return true
	default:
		return false
	}
}

// PendingCall is the durable control state of one tool call that has paused.
// It intentionally excludes executable runtime state; the matching Resume is
// attached to the tool's context when that call is continued.
type PendingCall struct {
	ID           string          `json:"id"`
	Reason       string          `json:"reason"`
	Prompt       json.RawMessage `json:"prompt"`
	ResumeSchema json.RawMessage `json:"resume_schema"`
}

// Validate verifies the stable resume identity and JSON protocol values.
func (p PendingCall) Validate() error {
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("%w: pending call ID must not be empty", ErrInvalidCheckpoint)
	}
	if strings.TrimSpace(p.Reason) == "" {
		return fmt.Errorf("%w: pending call reason must not be empty", ErrInvalidCheckpoint)
	}
	if !json.Valid(p.Prompt) || !json.Valid(p.ResumeSchema) {
		return fmt.Errorf("%w: pending call prompt and resume schema must be valid JSON", ErrInvalidCheckpoint)
	}
	return nil
}

// CallCheckpoint is the durable state of one response tool call. Result and
// Pending are mutually exclusive and selected by Status.
type CallCheckpoint struct {
	Status  CallStatus       `json:"status"`
	Result  *chat.ToolResult `json:"result,omitempty"`
	Pending *PendingCall     `json:"pending,omitempty"`
}

// Validate verifies the tagged payload independent of its response position.
func (c CallCheckpoint) Validate() error {
	if !c.Status.Valid() {
		return fmt.Errorf("%w: unknown call status %q", ErrInvalidCheckpoint, c.Status)
	}
	switch c.Status {
	case CallQueued:
		if c.Result != nil || c.Pending != nil {
			return fmt.Errorf("%w: queued call carries settled state", ErrInvalidCheckpoint)
		}
	case CallCompleted:
		if c.Result == nil || c.Pending != nil {
			return fmt.Errorf("%w: completed call requires only a result", ErrInvalidCheckpoint)
		}
		if err := c.Result.Validate(); err != nil {
			return fmt.Errorf("%w: completed result: %w", ErrInvalidCheckpoint, err)
		}
	case CallPaused:
		if c.Result != nil || c.Pending == nil {
			return fmt.Errorf("%w: paused call requires only pending state", ErrInvalidCheckpoint)
		}
		if err := c.Pending.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Checkpoint is the complete serializable state needed to continue a paused
// tool round without invoking the model again or re-running settled calls.
//
// CallStates follows the response's original tool-call order. NextResult is
// the first result not yet published to observers or appended to the
// continuation message; it always points at the currently exposed pause.
// Calls after it may already be completed or paused internally, but their
// externally visible commit waits for every earlier call. This stable ordering
// is required for deterministic cache inputs.
//
// MaxRounds and MaxConcurrentCalls are part of the persisted execution policy.
// Resume rejects a Runner configured differently instead of silently changing
// the schedule of work that remains queued.
//
// Checkpoint deliberately contains no executable ToolResolver; Resume receives
// that capability separately.
type Checkpoint struct {
	SchemaVersion      uint16           `json:"schema_version"`
	ID                 string           `json:"id"`
	Round              int              `json:"round"`
	MaxRounds          int              `json:"max_rounds"`
	MaxConcurrentCalls int              `json:"max_concurrent_calls"`
	ToolsetDigest      string           `json:"toolset_digest"`
	Request            *chat.Request    `json:"request"`
	Response           *chat.Response   `json:"response"`
	CallStates         []CallCheckpoint `json:"call_states"`
	NextResult         int              `json:"next_result"`
}

// Validate verifies the protocol snapshots and their call/result correlation.
func (c *Checkpoint) Validate() error {
	if c == nil {
		return fmt.Errorf("%w: nil receiver", ErrInvalidCheckpoint)
	}
	if c.SchemaVersion != CheckpointSchemaVersion {
		return fmt.Errorf("%w: unsupported schema version %d", ErrInvalidCheckpoint, c.SchemaVersion)
	}
	if strings.TrimSpace(c.ID) == "" {
		return fmt.Errorf("%w: ID must not be empty", ErrInvalidCheckpoint)
	}
	if c.Round < 1 {
		return fmt.Errorf("%w: round must be positive", ErrInvalidCheckpoint)
	}
	if c.MaxRounds < 1 || c.Round > c.MaxRounds {
		return fmt.Errorf("%w: round policy is inconsistent", ErrInvalidCheckpoint)
	}
	if c.MaxConcurrentCalls < 1 {
		return fmt.Errorf("%w: max concurrent calls must be positive", ErrInvalidCheckpoint)
	}
	if c.Request == nil {
		return fmt.Errorf("%w: request must not be nil", ErrInvalidCheckpoint)
	}
	if err := c.Request.Validate(); err != nil {
		return fmt.Errorf("%w: request: %w", ErrInvalidCheckpoint, err)
	}
	digest, err := toolsetDigest(c.Request.Tools)
	if err != nil {
		return fmt.Errorf("%w: toolset digest: %w", ErrInvalidCheckpoint, err)
	}
	if c.ToolsetDigest == "" || c.ToolsetDigest != digest {
		return fmt.Errorf("%w: toolset digest mismatch", ErrInvalidCheckpoint)
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
	if len(c.CallStates) != len(calls) {
		return fmt.Errorf("%w: %d call states do not match %d response calls", ErrInvalidCheckpoint, len(c.CallStates), len(calls))
	}
	if c.NextResult < 0 || c.NextResult >= len(calls) {
		return fmt.Errorf("%w: next result %d is outside [0,%d)", ErrInvalidCheckpoint, c.NextResult, len(calls))
	}

	queued := false
	started := 0
	for index, state := range c.CallStates {
		if err := state.Validate(); err != nil {
			return fmt.Errorf("%w: call_states[%d]: %w", ErrInvalidCheckpoint, index, err)
		}
		switch state.Status {
		case CallQueued:
			queued = true
		case CallCompleted:
			if queued {
				return fmt.Errorf("%w: call_states[%d] is completed after queued suffix began", ErrInvalidCheckpoint, index)
			}
			started++
			if state.Result.ID != calls[index].ID || state.Result.Name != calls[index].Name {
				return fmt.Errorf("%w: call_states[%d] result does not match tool call %q", ErrInvalidCheckpoint, index, calls[index].ID)
			}
		case CallPaused:
			if queued {
				return fmt.Errorf("%w: call_states[%d] is paused after queued suffix began", ErrInvalidCheckpoint, index)
			}
			started++
		}
	}
	if started == 0 {
		return fmt.Errorf("%w: checkpoint has no started calls", ErrInvalidCheckpoint)
	}
	for index := range c.NextResult {
		if c.CallStates[index].Status != CallCompleted {
			return fmt.Errorf("%w: call_states[%d] precedes next result but is not completed", ErrInvalidCheckpoint, index)
		}
	}
	active := c.CallStates[c.NextResult]
	if active.Status != CallPaused {
		return fmt.Errorf("%w: next result %d does not point at a paused call", ErrInvalidCheckpoint, c.NextResult)
	}
	if active.Pending.ID != c.ID {
		return fmt.Errorf("%w: checkpoint ID %q does not match active pending call %q", ErrInvalidCheckpoint, c.ID, active.Pending.ID)
	}
	return nil
}

// ToolCalls returns the checkpoint's canonical response calls in model order.
// The returned slice is independent of the durable checkpoint.
func (c *Checkpoint) ToolCalls() ([]chat.ToolCall, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	calls, err := responseToolCalls(c.Response)
	if err != nil {
		return nil, fmt.Errorf("%w: response: %w", ErrInvalidCheckpoint, err)
	}
	return append([]chat.ToolCall(nil), calls...), nil
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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("%w: decode: %w", ErrInvalidCheckpoint, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: trailing JSON value", ErrInvalidCheckpoint)
	}
	candidate := Checkpoint(decoded)
	if err := candidate.Validate(); err != nil {
		return err
	}
	*c = candidate
	return nil
}

func cloneCallStates(states []CallCheckpoint) []CallCheckpoint {
	if states == nil {
		return nil
	}
	cloned := make([]CallCheckpoint, len(states))
	for index, state := range states {
		cloned[index].Status = state.Status
		if state.Result != nil {
			result := *state.Result
			cloned[index].Result = &result
		}
		if state.Pending != nil {
			pending := *state.Pending
			pending.Prompt = bytes.Clone(state.Pending.Prompt)
			pending.ResumeSchema = bytes.Clone(state.Pending.ResumeSchema)
			cloned[index].Pending = &pending
		}
	}
	return cloned
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
