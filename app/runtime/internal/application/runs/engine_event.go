package runs

import (
	"errors"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

// ExecutorSource is the executor-owned identity of the process that produced an
// event. It deliberately carries no Run or Segment identity: mapping execution
// processes onto application Runs belongs to the Coordinator.
type ExecutorSource struct {
	ProcessID   string
	ParentID    string
	SpawnCallID string
}

// Child reports whether this source belongs to a delegated process.
func (source ExecutorSource) Child() bool { return source.ParentID != "" }

// Validate rejects malformed or self-referential process identity. An entirely
// empty source is reserved for a root turn that failed before the executor
// created its process.
func (source ExecutorSource) Validate() error {
	if source.ProcessID != strings.TrimSpace(source.ProcessID) {
		return errors.New("runs: executor source process id has surrounding whitespace")
	}
	if source.ParentID != strings.TrimSpace(source.ParentID) {
		return errors.New("runs: executor source parent id has surrounding whitespace")
	}
	if source.SpawnCallID != strings.TrimSpace(source.SpawnCallID) {
		return errors.New("runs: executor source spawn call id has surrounding whitespace")
	}
	if source.ProcessID == "" {
		if source.ParentID != "" || source.SpawnCallID != "" {
			return errors.New("runs: empty executor process id cannot carry parent or spawn-call identity")
		}
		return nil
	}
	if source.ParentID == source.ProcessID {
		return errors.New("runs: executor source cannot parent itself")
	}
	if source.ParentID == "" && source.SpawnCallID != "" {
		return errors.New("runs: root executor source cannot carry spawn-call identity")
	}
	return nil
}

// ExecutorEvent is the driven-executor port value. Source identifies the
// concrete root/child process; Payload is the closed application-owned signal
// family. A root stream may therefore carry child signals without relabeling
// their producer as the root.
type ExecutorEvent struct {
	Source  ExecutorSource
	Payload ExecutorPayload
}

// Validate checks the envelope before the Coordinator routes it.
func (event ExecutorEvent) Validate() error {
	if event.Payload == nil {
		return errors.New("runs: executor event payload is required")
	}
	return event.Source.Validate()
}

// ExecutorPayload is the closed family carried by the ordered executor stream.
// Most values are reducible [EngineEvent] facts. A control handshake such as
// [ChildOpeningRequest] shares the stream only when its ordering relative to
// those facts is itself part of correctness.
type ExecutorPayload interface {
	executorPayload()
}

type executorPayloadBase struct{}

func (executorPayloadBase) executorPayload() {}

// EngineEvent is the closed application-owned execution fact family. Driven
// adapters emit these values at the SegmentExecutor port; delivery therefore
// projects an application contract and never reaches into an executor adapter.
// Control handshakes deliberately implement only [ExecutorPayload], so they
// cannot accidentally enter the reducer.
type EngineEvent interface {
	ExecutorPayload
	engineEvent()
}

type engineEventBase struct{ executorPayloadBase }

func (engineEventBase) engineEvent() {}

type MessageDelta struct {
	engineEventBase
	Text string
}

type ReasoningDelta struct {
	engineEventBase
	Text string
}

type ToolCallStart struct {
	engineEventBase
	CallID string
	// SourceCallID is the executor's parent-call identity. It never reaches the
	// protocol; it exists solely to map a child Process causal edge to this
	// canonical Item without parsing CallID or relying on event timing.
	SourceCallID string
	ToolName     string
	Arguments    string
	Activity     string
	SafetyClass  tool.SafetyClass
}

type ToolCallEnd struct {
	engineEventBase
	CallID       string
	Arguments    string
	Result       *tool.Result
	Offload      *offload.Ref
	OutputText   string
	MutatedPaths []string
	Err          string
	Denied       bool
}

// FileChange is a live workspace refresh nudge emitted after a tool-owned file
// mutation commits. Delivery only encodes these already-resolved values.
type FileChange struct {
	Cwd   string
	Paths []string
}

type CompactBoundary struct {
	engineEventBase
	MessagesBefore int
	MessagesAfter  int
}

type TurnInterrupted struct {
	engineEventBase
	Interrupts []Interrupt
	// Duration is how long this segment executed before parking. A parked Run
	// still reports what it consumed, so the executor stamps it here for the same
	// reason it stamps it on TurnEnd.
	Duration time.Duration
}

func (e TurnInterrupted) validate() error {
	if len(e.Interrupts) == 0 {
		return errors.New("runs: executor emitted an empty interrupt")
	}
	for _, interrupt := range e.Interrupts {
		if err := interrupt.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type TurnEnd struct {
	engineEventBase
	Reason execution.Outcome
	// Problem is present exactly when Reason is OutcomeError. It is already a
	// stable, client-safe application problem; executor diagnostics never enter
	// the event stream.
	Problem *transcript.Problem
	// Usage is the segment's final accounting, and is absent when the terminal
	// produced none — a cancellation or a failure joins without a TurnOutput to
	// account for. Absent is NOT zero: reading a missing report as "spent
	// nothing" made a canceled Run's committed metering fall back below what its
	// own progress events had already published.
	Usage    *TurnUsage
	Duration time.Duration
}

// TurnUsage is one authoritative accounting report for a segment. The three
// numbers are produced together by the executor's observer, so they travel
// together: a report that had tokens but no per-model split would be a different
// report, not this one with a field missing.
type TurnUsage struct {
	Tokens  accounting.TokenUsage
	ByModel []accounting.ModelUsage
	CostUSD float64
}

type UsageReported struct {
	engineEventBase
	TokenUsage    accounting.TokenUsage
	CostUSD       float64
	ContextTokens int64
}

// TodosUpdated reports the committed task list after a replacement — read back
// from the store, so what is published is what was written rather than what the
// tool was asked to write.
type TodosUpdated struct {
	engineEventBase
	State todo.State
}

type SteerMessage struct {
	engineEventBase
	Content []transcript.ContentBlock
}
