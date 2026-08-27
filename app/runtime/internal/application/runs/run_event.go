package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
)

type RunEvent interface {
	runEvent()
	Replayable() bool
	Terminal() bool
	// retainedBytes reports the approximate heap retained by this value. Keeping
	// it on the closed event family makes retention accounting a compile-time
	// obligation whenever a new event variant is introduced.
	retainedBytes() int
}

type SegmentStarted struct{ Run run.Run }
type SegmentProgressed struct{ Progress RunProgress }
type SegmentFinished struct {
	Run        run.Run
	Interrupts []transcript.Interrupt
}

// ItemStart is the provisional stream projection emitted before a complete
// transcript fact exists. It is deliberately an Application value: message and
// reasoning deltas need a rendering anchor, but they are not running Domain
// Items. Only a running ToolCall may carry a durable Item into a waiting
// boundary.
type ItemStart struct {
	SessionID      string
	RunID          string
	ItemID         string
	Kind           transcript.ItemKind
	OccurredAt     time.Time
	ToolInvocation *transcript.ToolInvocation
	SafetyClass    tool.SafetyClass

	durable *transcript.Item
}

type ItemStarted struct{ Item ItemStart }
type ItemChanged struct {
	ItemID string
	Delta  ItemDelta
}
type ItemCompleted struct {
	Item         transcript.Item
	mutatedPaths []string
}

// PlanSnapshot publishes a persisted latest-value projection the run changed. It
// carries the projection's own revision, not just its contents: the list is
// replaced wholesale, so a fold that only saw contents could not tell an older
// snapshot from a newer one.
type PlanSnapshot struct {
	SessionID string
	Steps     []plan.Step
	Revision  uint64
	UpdatedAt time.Time
}

func (SegmentStarted) runEvent()    {}
func (SegmentProgressed) runEvent() {}
func (SegmentFinished) runEvent()   {}
func (ItemStarted) runEvent()       {}
func (ItemChanged) runEvent()       {}
func (ItemCompleted) runEvent()     {}
func (PlanSnapshot) runEvent()      {}

func (SegmentStarted) Replayable() bool    { return true }
func (SegmentProgressed) Replayable() bool { return false }
func (SegmentFinished) Replayable() bool   { return true }
func (ItemStarted) Replayable() bool       { return true }
func (ItemChanged) Replayable() bool       { return false }
func (ItemCompleted) Replayable() bool     { return true }
func (PlanSnapshot) Replayable() bool      { return true }

func (SegmentStarted) Terminal() bool    { return false }
func (SegmentProgressed) Terminal() bool { return false }
func (SegmentFinished) Terminal() bool   { return true }
func (ItemStarted) Terminal() bool       { return false }
func (ItemChanged) Terminal() bool       { return false }
func (ItemCompleted) Terminal() bool     { return false }
func (PlanSnapshot) Terminal() bool      { return false }

func (s SegmentStarted) retainedBytes() int  { return retainedRunBytes(s.Run) }
func (SegmentProgressed) retainedBytes() int { return 0 }
func (s SegmentFinished) retainedBytes() int {
	bytes := retainedRunBytes(s.Run) + cap(s.Interrupts)*retainedInterruptBytes
	for _, pending := range s.Interrupts {
		bytes += retainedInterruptPayloadBytes(pending)
	}
	return bytes
}
func (i ItemStarted) retainedBytes() int   { return retainedItemStartBytes(i.Item) }
func (ItemChanged) retainedBytes() int     { return 0 }
func (i ItemCompleted) retainedBytes() int { return retainedItemBytes(i.Item) }
func (p PlanSnapshot) retainedBytes() int  { return retainedPlanSnapshotBytes(p) }

type RunProgress struct {
	Step          *int
	Usage         *accounting.Usage
	ContextTokens *int64
	ToolName      string
	Activity      string
}

type ItemDeltaKind string

const (
	ContentDelta       ItemDeltaKind = "content"
	ReasoningDeltaKind ItemDeltaKind = "reasoning"
	ToolArgumentsDelta ItemDeltaKind = "toolArguments"
	ToolOutputDelta    ItemDeltaKind = "toolOutput"
)

func (i ItemDeltaKind) Valid() bool {
	return i == ContentDelta || i == ReasoningDeltaKind || i == ToolArgumentsDelta || i == ToolOutputDelta
}

type ItemDelta struct {
	Kind               ItemDeltaKind
	Index              *int
	Text               string
	ArgumentsTextDelta string
}

func newTransientItemStart(identity transcript.ItemIdentity, kind transcript.ItemKind) (ItemStart, error) {
	if err := identity.Validate(); err != nil {
		return ItemStart{}, err
	}
	if kind != transcript.AgentMessage && kind != transcript.Reasoning {
		return ItemStart{}, fmt.Errorf("runs: Item start kind %q is not a transient stream", kind)
	}
	return ItemStart{
		SessionID: identity.SessionID, RunID: identity.RunID, ItemID: identity.ItemID,
		Kind: kind, OccurredAt: identity.OccurredAt,
	}, nil
}

func newToolItemStart(item transcript.Item) (ItemStart, error) {
	if item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemRunning {
		return ItemStart{}, errors.New("runs: durable Item start is not a running ToolCall")
	}
	invocation, ok := item.ToolInvocation()
	if !ok {
		return ItemStart{}, errors.New("runs: running ToolCall has no invocation")
	}
	owned := item
	return ItemStart{
		SessionID: item.SessionID(), RunID: item.RunID(), ItemID: item.ID(),
		Kind: item.Kind(), OccurredAt: item.OccurredAt(), ToolInvocation: &invocation,
		SafetyClass: item.SafetyClass(), durable: &owned,
	}, nil
}

func (i ItemStart) validate() error {
	if err := (transcript.ItemIdentity{
		SessionID: i.SessionID, RunID: i.RunID, ItemID: i.ItemID,
		OccurredAt: i.OccurredAt,
	}).Validate(); err != nil {
		return err
	}
	if i.Kind != transcript.AgentMessage && i.Kind != transcript.Reasoning && i.Kind != transcript.ToolCall {
		return fmt.Errorf("runs: unsupported Item start kind %q", i.Kind)
	}
	if i.Kind != transcript.ToolCall {
		if i.ToolInvocation != nil || i.SafetyClass != "" || i.durable != nil {
			return errors.New("runs: transient Item start carries ToolCall facts")
		}
		return nil
	}
	if i.ToolInvocation == nil || strings.TrimSpace(i.ToolInvocation.Name) == "" || i.durable == nil {
		return errors.New("runs: ToolCall start has no durable invocation")
	}
	item := *i.durable
	if item.SessionID() != i.SessionID || item.RunID() != i.RunID ||
		item.ID() != i.ItemID || item.Kind() != i.Kind ||
		!item.OccurredAt().Equal(i.OccurredAt) {
		return errors.New("runs: ToolCall start differs from its durable Item")
	}
	invocation, present := item.ToolInvocation()
	if !present || invocation.Name != i.ToolInvocation.Name ||
		invocation.Arguments != i.ToolInvocation.Arguments ||
		item.SafetyClass() != i.SafetyClass {
		return errors.New("runs: ToolCall start differs from its durable invocation")
	}
	return item.Validate()
}
