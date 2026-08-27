package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/scope/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/toolresult"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
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
func (e ExecutorMember) Child() bool { return e.ParentID != "" }

// Validate rejects malformed or self-referential member identity. An entirely
// empty member is reserved for a root execution that failed before the executor
// created its member.
func (e ExecutorMember) Validate() error {
	if e.MemberID != strings.TrimSpace(e.MemberID) {
		return errors.New("runs: executor member id has surrounding whitespace")
	}
	if e.ParentID != strings.TrimSpace(e.ParentID) {
		return errors.New("runs: executor member parent id has surrounding whitespace")
	}
	if e.SpawnCallID != strings.TrimSpace(e.SpawnCallID) {
		return errors.New("runs: executor member spawn call id has surrounding whitespace")
	}
	if e.MemberID == "" {
		if e.ParentID != "" || e.SpawnCallID != "" {
			return errors.New("runs: empty executor member id cannot carry parent or spawn-call identity")
		}
		return nil
	}
	if e.ParentID == e.MemberID {
		return errors.New("runs: executor member cannot parent itself")
	}
	if e.ParentID == "" && e.SpawnCallID != "" {
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
func (e ExecutorEvent) Validate() error {
	if e.Payload == nil {
		return errors.New("runs: executor event payload is required")
	}
	return e.Member.Validate()
}

// ExecutorPayload is the closed family carried by the ordered executor stream.
// Most values are reducible [ExecutionFact] facts. A control handshake such as
// control requests share the stream only when their ordering relative to those
// facts is itself part of correctness.
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

// AssistantMessageCompleted confirms that the executor's final process output
// is the same assistant message already committed at the authoritative
// [ModelCallCompleted] boundary. It may race ahead of that boundary, so the
// reducer buffers it until the model fact arrives; it never creates a second
// transcript Item.
type AssistantMessageCompleted struct {
	executionFactBase
	Message corechat.Message
}

// UnknownEffectsDetected is the executor's fail-closed control observation that
// one or more externally attempted Effects have no provable settlement. IDs are
// opaque diagnostics. The Run pump—not the executor—maps this condition to the
// product's RunLost transaction before resource teardown.
type UnknownEffectsDetected struct {
	executorPayloadBase
	IDs []string
}

func (u UnknownEffectsDetected) validate() error {
	if len(u.IDs) == 0 {
		return errors.New("runs: unknown Effect observation is empty")
	}
	previous := ""
	for index, id := range u.IDs {
		if strings.TrimSpace(id) == "" || id != strings.TrimSpace(id) {
			return fmt.Errorf("runs: unknown Effect id[%d] is invalid", index)
		}
		if index > 0 && id <= previous {
			return errors.New("runs: unknown Effect ids must be unique and sorted")
		}
		previous = id
	}
	return nil
}

// ModelCallStarted is the authoritative pre-provider boundary for one model
// invocation. CallID is an opaque, stable executor identity; the Application
// never parses framework Effect identity from it.
type ModelCallStarted struct {
	executionFactBase
	CallID string
}

// ModelCallCompleted is the sole authoritative semantic and accounting projection
// of one completed model invocation. Message may contain ToolCall parts; the
// reducer projects only assistant content/reasoning here because each actual
// Tool invocation has its own pre-call commit boundary.
type ModelCallCompleted struct {
	executionFactBase
	CallID        string
	Message       corechat.Message
	TokenUsage    accounting.TokenUsage
	ByModel       []accounting.ModelUsage
	CostUSD       float64
	Steps         int
	ContextTokens int64
}

// ModelCallFailed closes a provider attempt whose failure is definite. It has
// no semantic assistant output or usage. If this fact itself cannot be durably
// committed after the provider was called, the dispatcher must instead leave
// the invocation open and reconcile the Effect as unknown.
type ModelCallFailed struct {
	executionFactBase
	CallID string
}

type ToolCallStarted struct {
	executionFactBase
	CallID            string
	ModelCallSequence uint32
	ToolCallIndex     uint32
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
	// Failure is the one structured failure channel for a completed Tool call.
	// nil means success; its Tool taxonomy prevents Run failures from entering
	// this slot without parallel error strings or boolean classifications.
	Failure *tool.Failure
}

type CompactionBoundary struct {
	executionFactBase
	MessagesBefore int
	MessagesAfter  int
}

// MemberInterruption is one direct external-input boundary discovered in the
// executor tree. MemberID identifies the member already admitted to an
// application Run; RequestID is private continuation identity; Interrupt is
// the application prompt projected from that request.
type MemberInterruption struct {
	MemberID  string
	RequestID string
	Interrupt Interrupt
}

// TreeInterrupted is the executor's complete, stable view of one human-input
// barrier. It is a control payload rather than an ExecutionFact because no single
// Run reducer may commit it: the Coordinator must suspend the entire active tree
// in one transaction.
type TreeInterrupted struct {
	executorPayloadBase
	// Checkpoint is the immutable executor state captured at the same waiting
	// boundary as Interruptions. The Coordinator never interprets it; it only
	// places its write into the tree-barrier transaction.
	Checkpoint    ExecutorCheckpoint
	Interruptions []MemberInterruption
}

func (t TreeInterrupted) validate() error {
	if err := t.Checkpoint.Validate(); err != nil {
		return fmt.Errorf("runs: executor tree interrupt has an invalid checkpoint: %w", err)
	}
	if len(t.Interruptions) == 0 {
		return errors.New("runs: executor emitted an empty tree interrupt")
	}
	seen := make(map[string]struct{}, len(t.Interruptions))
	for index, request := range t.Interruptions {
		if strings.TrimSpace(request.MemberID) == "" {
			return fmt.Errorf("runs: tree interrupt request[%d] has no member id", index)
		}
		if strings.TrimSpace(request.RequestID) == "" {
			return fmt.Errorf("runs: tree interrupt request[%d] has no request id", index)
		}
		if err := request.Interrupt.Validate(); err != nil {
			return fmt.Errorf("runs: tree interrupt request[%d]: %w", index, err)
		}
		key := request.MemberID + "\x00" + request.RequestID
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"runs: member %q repeated request %q",
				request.MemberID,
				request.RequestID,
			)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func (t TreeInterrupted) validateFor(
	rootMemberID string,
	sessionID string,
	goalIncarnationID string,
	selection modelref.Selection,
) error {
	if err := t.validate(); err != nil {
		return err
	}
	if err := t.Checkpoint.ValidateOwnership(rootMemberID, sessionID); err != nil {
		return fmt.Errorf("runs: executor tree interrupt checkpoint ownership: %w", err)
	}
	if t.Checkpoint.Scope.GoalIncarnationID != goalIncarnationID {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint goal incarnation %q does not match Run %q: %w",
			t.Checkpoint.Scope.GoalIncarnationID,
			goalIncarnationID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if t.Checkpoint.ModelSelection != selection {
		return fmt.Errorf(
			"runs: executor tree interrupt checkpoint model %q/%q does not match Run %q/%q: %w",
			t.Checkpoint.ModelSelection.Provider(),
			t.Checkpoint.ModelSelection.Model(),
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

func (s SegmentInterrupted) validate() error {
	if len(s.Interrupts) == 0 {
		return errors.New("runs: executor emitted an empty interrupt")
	}
	for _, interrupt := range s.Interrupts {
		if err := interrupt.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type SegmentEnded struct {
	executionFactBase
	Reason run.Outcome
	// Failure is present exactly when Reason is Failed, TimedOut, or Lost. It is
	// already a stable, client-safe classification; executor diagnostics
	// never enter the event stream.
	Failure *run.Failure
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

// SteerMessagesApplied reports the ordered user messages first made visible
// to one executor model call. The reducer commits the complete batch atomically
// so transcript projection cannot expose a prefix of one model boundary.
type SteerMessagesApplied struct {
	executionFactBase
	Messages [][]transcript.ContentBlock
}
