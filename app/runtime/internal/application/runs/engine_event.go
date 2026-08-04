package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
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
// empty source is reserved for a root execution that failed before the executor
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

// EngineEvent is the closed application-owned execution fact family emitted at
// the SegmentExecutor port. Control handshakes deliberately implement only
// [ExecutorPayload], so they cannot accidentally enter the reducer.
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
	// SourceCallID is the executor's parent-call identity. It exists solely to map
	// a child Process causal edge to this
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
	// Problem is the one structured failure channel for a completed tool call.
	// nil means success; the problem's Tool scope distinguishes it from a Run
	// failure without parallel error strings or boolean classifications.
	Problem *transcript.Problem
}

type CompactBoundary struct {
	engineEventBase
	MessagesBefore int
	MessagesAfter  int
}

// ProcessSuspension is one direct external-input boundary discovered in the
// executor tree. ProcessID identifies the source process already admitted to an
// application Run; SuspensionID is private continuation identity; Interrupt is
// the application prompt projected from that suspension.
type ProcessSuspension struct {
	ProcessID    string
	SuspensionID string
	Interrupt    Interrupt
}

// TreeInterrupted is the executor's complete, stable view of one human-input
// barrier. It is a control payload rather than an EngineEvent because no single
// Run reducer may commit it: the Coordinator must suspend the entire active tree
// in one transaction.
type TreeInterrupted struct {
	executorPayloadBase
	// Checkpoint is the immutable executor state captured at the same waiting
	// boundary as Suspensions. The Coordinator never interprets it; it only
	// places its write into the tree-barrier transaction.
	Checkpoint  execution.ExecutorCheckpoint
	Suspensions []ProcessSuspension
}

func (barrier TreeInterrupted) validate() error {
	if err := barrier.Checkpoint.Validate(); err != nil {
		return fmt.Errorf("runs: executor tree interrupt has an invalid checkpoint: %w", err)
	}
	if len(barrier.Suspensions) == 0 {
		return errors.New("runs: executor emitted an empty tree interrupt")
	}
	seen := make(map[string]struct{}, len(barrier.Suspensions))
	for index, suspension := range barrier.Suspensions {
		if strings.TrimSpace(suspension.ProcessID) == "" {
			return fmt.Errorf("runs: tree interrupt suspension[%d] has no process id", index)
		}
		if strings.TrimSpace(suspension.SuspensionID) == "" {
			return fmt.Errorf("runs: tree interrupt suspension[%d] has no suspension id", index)
		}
		if err := suspension.Interrupt.Validate(); err != nil {
			return fmt.Errorf("runs: tree interrupt suspension[%d]: %w", index, err)
		}
		key := suspension.ProcessID + "\x00" + suspension.SuspensionID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"runs: process %q repeated suspension %q",
				suspension.ProcessID,
				suspension.SuspensionID,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (barrier TreeInterrupted) validateFor(
	rootProcessID string,
	sessionID string,
	goalLeaseID string,
	selection modelref.Selection,
) error {
	if err := barrier.validate(); err != nil {
		return err
	}
	if err := barrier.Checkpoint.ValidateOwnership(rootProcessID, sessionID); err != nil {
		return fmt.Errorf("runs: executor tree interrupt checkpoint ownership: %w", err)
	}
	if barrier.Checkpoint.Scope.GoalLeaseID != goalLeaseID {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint goal lease %q does not match Run %q: %w",
			barrier.Checkpoint.Scope.GoalLeaseID,
			goalLeaseID,
			execution.ErrInvalidExecutorCheckpoint,
		)
	}
	if barrier.Checkpoint.ModelSelection != selection {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint model %q/%q does not match Run %q/%q: %w",
			barrier.Checkpoint.ModelSelection.Provider(),
			barrier.Checkpoint.ModelSelection.Model(),
			selection.Provider(),
			selection.Model(),
			execution.ErrInvalidExecutorCheckpoint,
		)
	}
	return nil
}

// SegmentInterrupted is the source-Run reducer input derived from a
// [TreeInterrupted] barrier. Executor adapters never emit it directly.
type SegmentInterrupted struct {
	engineEventBase
	Interrupts []Interrupt
	// Duration is how long this segment executed before parking. A parked Run
	// still reports what it consumed, so the executor stamps it here for the same
	// reason it stamps it on SegmentEnded.
	Duration time.Duration
}

func (e SegmentInterrupted) validate() error {
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

type SegmentEnded struct {
	engineEventBase
	Reason execution.Outcome
	// Problem is present exactly when Reason is OutcomeError. It is already a
	// stable, client-safe application problem; executor diagnostics never enter
	// the event stream.
	Problem *transcript.Problem
	// Usage is the segment's final accounting, and is absent only when the
	// executor cannot produce an authoritative report. Child executors retain
	// their subtree ledger through cancellation and failure, so those terminals
	// can still carry usage. Absent is NOT zero: reading a missing report as
	// "spent nothing" made a canceled Run's committed metering fall back below
	// what its own progress events had already published.
	Usage    *SegmentUsage
	Duration time.Duration
}

// SegmentUsage is one authoritative accounting report for a segment. The three
// numbers are produced together by the executor's observer, so they travel
// together: a report that had tokens but no per-model split would be a different
// report, not this one with a field missing.
type SegmentUsage struct {
	Tokens  accounting.TokenUsage
	ByModel []accounting.ModelUsage
	CostUSD float64
	Steps   int
}

type UsageReported struct {
	engineEventBase
	TokenUsage    accounting.TokenUsage
	ByModel       []accounting.ModelUsage
	CostUSD       float64
	Steps         int
	ContextTokens int64
}

// PlanUpdated reports the committed Plan after a replacement — read back
// from the store, so what is published is what was written rather than what the
// tool was asked to write.
type PlanUpdated struct {
	engineEventBase
	State plan.State
}

type SteerMessage struct {
	engineEventBase
	Content []transcript.ContentBlock
}
