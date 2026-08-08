package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/lynx/core/chat"
)

// ExecutorMember is the executor-owned identity of the member that produced an
// event. It deliberately carries no Run or Segment identity: mapping executor
// members onto application Runs belongs to the Coordinator.
type ExecutorMember struct {
	MemberID    string
	ParentID    string
	SpawnCallID string
}

// Child reports whether this member was delegated by another member.
func (member ExecutorMember) Child() bool { return member.ParentID != "" }

// Validate rejects malformed or self-referential member identity. An entirely
// empty member is reserved for a root execution that failed before the executor
// created its member.
func (member ExecutorMember) Validate() error {
	if member.MemberID != strings.TrimSpace(member.MemberID) {
		return errors.New("runs: executor member id has surrounding whitespace")
	}
	if member.ParentID != strings.TrimSpace(member.ParentID) {
		return errors.New("runs: executor member parent id has surrounding whitespace")
	}
	if member.SpawnCallID != strings.TrimSpace(member.SpawnCallID) {
		return errors.New("runs: executor member spawn call id has surrounding whitespace")
	}
	if member.MemberID == "" {
		if member.ParentID != "" || member.SpawnCallID != "" {
			return errors.New("runs: empty executor member id cannot carry parent or spawn-call identity")
		}
		return nil
	}
	if member.ParentID == member.MemberID {
		return errors.New("runs: executor member cannot parent itself")
	}
	if member.ParentID == "" && member.SpawnCallID != "" {
		return errors.New("runs: root executor member cannot carry spawn-call identity")
	}
	return nil
}

// ExecutorEvent is the driven-executor port value. Member identifies the
// concrete root/child member; Payload is the closed application-owned signal
// family. A root stream may therefore carry child signals without relabeling
// their producer as the root.
type ExecutorEvent struct {
	Member  ExecutorMember
	Payload ExecutorPayload
}

// Validate checks the envelope before the Coordinator routes it.
func (event ExecutorEvent) Validate() error {
	if event.Payload == nil {
		return errors.New("runs: executor event payload is required")
	}
	return event.Member.Validate()
}

// ExecutorPayload is the closed family carried by the ordered executor stream.
// Most values are reducible [ExecutionFact] facts. A control handshake such as
// [ChildOpeningRequest] shares the stream only when its ordering relative to
// those facts is itself part of correctness.
type ExecutorPayload interface {
	executorPayload()
}

type executorPayloadBase struct{}

func (executorPayloadBase) executorPayload() {}

// ExecutionFact is the closed application-owned execution fact family emitted at
// the ExecutionObserver port. Control handshakes deliberately implement only
// [ExecutorPayload], so they cannot accidentally enter the reducer.
type ExecutionFact interface {
	ExecutorPayload
	executionFact()
}

type executionFactBase struct{ executorPayloadBase }

func (executionFactBase) executionFact() {}

type MessageDelta struct {
	executionFactBase
	Text string
}

type ReasoningDelta struct {
	executionFactBase
	Text string
}

// AssistantMessageCompleted is the authoritative semantic assistant message
// produced by an executor. Unlike [MessageDelta] and [ReasoningDelta], it is a
// complete final value and remains correct when streaming observation is
// disabled or drops increments.
type AssistantMessageCompleted struct {
	executionFactBase
	Message corechat.Message
}

type ToolCallStarted struct {
	executionFactBase
	CallID string
	// SourceCallID is the executor's parent-call identity. It exists solely to map
	// a child member causal edge to this
	// canonical Item without parsing CallID or relying on event timing.
	SourceCallID string
	ToolName     string
	Arguments    string
	Activity     string
	SafetyClass  tool.SafetyClass
}

type ToolCallFinished struct {
	executionFactBase
	CallID       string
	Arguments    string
	Result       *tool.Result
	Offload      *toolresult.Ref
	OutputText   string
	MutatedPaths []string
	// Problem is the one structured failure channel for a completed tool call.
	// nil means success; the problem's Tool scope distinguishes it from a Run
	// failure without parallel error strings or boolean classifications.
	Problem *transcript.Problem
}

type CompactionBoundary struct {
	executionFactBase
	MessagesBefore int
	MessagesAfter  int
}

// MemberInterruption is one direct external-input boundary discovered in the
// executor tree. MemberID identifies the member already admitted to an
// application Run; SuspensionID is private continuation identity; Interrupt is
// the application prompt projected from that suspension.
type MemberInterruption struct {
	MemberID     string
	SuspensionID string
	Interrupt    Interrupt
}

// TreeInterrupted is the executor's complete, stable view of one human-input
// barrier. It is a control payload rather than an ExecutionFact because no single
// Run reducer may commit it: the Coordinator must suspend the entire active tree
// in one transaction.
type TreeInterrupted struct {
	executorPayloadBase
	// Checkpoint is the immutable executor state captured at the same waiting
	// boundary as Suspensions. The Coordinator never interprets it; it only
	// places its write into the tree-barrier transaction.
	Checkpoint  ExecutorCheckpoint
	Suspensions []MemberInterruption
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
		if strings.TrimSpace(suspension.MemberID) == "" {
			return fmt.Errorf("runs: tree interrupt suspension[%d] has no member id", index)
		}
		if strings.TrimSpace(suspension.SuspensionID) == "" {
			return fmt.Errorf("runs: tree interrupt suspension[%d] has no suspension id", index)
		}
		if err := suspension.Interrupt.Validate(); err != nil {
			return fmt.Errorf("runs: tree interrupt suspension[%d]: %w", index, err)
		}
		key := suspension.MemberID + "\x00" + suspension.SuspensionID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"runs: member %q repeated suspension %q",
				suspension.MemberID,
				suspension.SuspensionID,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (barrier TreeInterrupted) validateFor(
	rootMemberID string,
	sessionID string,
	goalLeaseID string,
	selection modelref.Selection,
) error {
	if err := barrier.validate(); err != nil {
		return err
	}
	if err := barrier.Checkpoint.ValidateOwnership(rootMemberID, sessionID); err != nil {
		return fmt.Errorf("runs: executor tree interrupt checkpoint ownership: %w", err)
	}
	if barrier.Checkpoint.Scope.GoalLeaseID != goalLeaseID {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint goal lease %q does not match Run %q: %w",
			barrier.Checkpoint.Scope.GoalLeaseID,
			goalLeaseID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if barrier.Checkpoint.ModelSelection != selection {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint model %q/%q does not match Run %q/%q: %w",
			barrier.Checkpoint.ModelSelection.Provider(),
			barrier.Checkpoint.ModelSelection.Model(),
			selection.Provider(),
			selection.Model(),
			ErrInvalidExecutorCheckpoint,
		)
	}
	return nil
}

// SegmentInterrupted is the member-Run reducer input derived from a
// [TreeInterrupted] barrier. Executor sources never emit it directly.
type SegmentInterrupted struct {
	executionFactBase
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
	executionFactBase
	Reason run.Outcome
	// Problem is present exactly when Reason is OutcomeFailed. It is already a
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
	executionFactBase
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
	executionFactBase
	State plan.State
}

type SteerMessage struct {
	executionFactBase
	Content []transcript.ContentBlock
}
