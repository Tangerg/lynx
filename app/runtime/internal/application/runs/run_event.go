package runs

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
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

// StateSnapshot publishes a persisted latest-value projection the run changed. It
// carries the projection's own revision, not just its contents: the list is
// replaced wholesale, so a fold that only saw contents could not tell an older
// snapshot from a newer one.
type StateSnapshot struct {
	SessionID string
	Plan      []PlanSnapshot
	Revision  uint64
	UpdatedAt time.Time
}

func (SegmentStarted) runEvent()    {}
func (SegmentProgressed) runEvent() {}
func (SegmentFinished) runEvent()   {}
func (ItemStarted) runEvent()       {}
func (ItemChanged) runEvent()       {}
func (ItemCompleted) runEvent()     {}
func (StateSnapshot) runEvent()     {}

func (SegmentStarted) Replayable() bool    { return true }
func (SegmentProgressed) Replayable() bool { return false }
func (SegmentFinished) Replayable() bool   { return true }
func (ItemStarted) Replayable() bool       { return true }
func (ItemChanged) Replayable() bool       { return false }
func (ItemCompleted) Replayable() bool     { return true }
func (StateSnapshot) Replayable() bool     { return true }

func (SegmentStarted) Terminal() bool    { return false }
func (SegmentProgressed) Terminal() bool { return false }
func (SegmentFinished) Terminal() bool   { return true }
func (ItemStarted) Terminal() bool       { return false }
func (ItemChanged) Terminal() bool       { return false }
func (ItemCompleted) Terminal() bool     { return false }
func (StateSnapshot) Terminal() bool     { return false }

func (event SegmentStarted) retainedBytes() int { return retainedRunBytes(event.Run) }
func (SegmentProgressed) retainedBytes() int    { return 0 }
func (event SegmentFinished) retainedBytes() int {
	bytes := retainedRunBytes(event.Run) + cap(event.Interrupts)*retainedInterruptBytes
	for _, pending := range event.Interrupts {
		bytes += retainedInterruptPayloadBytes(pending)
	}
	return bytes
}
func (event ItemStarted) retainedBytes() int   { return retainedItemStartBytes(event.Item) }
func (ItemChanged) retainedBytes() int         { return 0 }
func (event ItemCompleted) retainedBytes() int { return retainedItemBytes(event.Item) }
func (event StateSnapshot) retainedBytes() int { return retainedStateSnapshotBytes(event) }

type RunProgress struct {
	Step          *int
	Usage         *accounting.Usage
	ContextTokens *int64
	ToolName      string
	Activity      string
}

type ItemDeltaKind uint8

const (
	ContentDelta ItemDeltaKind = iota
	ReasoningDeltaKind
	ToolArgumentsDelta
	ToolOutputDelta
)

type ItemDelta struct {
	Kind               ItemDeltaKind
	Index              *int
	Text               string
	ArgumentsTextDelta string
}

type PlanSnapshot struct {
	ID          string
	Description string
	Status      plan.Status
}

func newTransientItemStart(identity transcript.ItemIdentity, kind transcript.ItemKind) (ItemStart, error) {
	if err := identity.Validate(); err != nil {
		return ItemStart{}, err
	}
	if kind != transcript.AgentMessage && kind != transcript.Reasoning {
		return ItemStart{}, fmt.Errorf("runs: Item start kind %d is not a transient stream", kind)
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

func (start ItemStart) validate() error {
	if err := (transcript.ItemIdentity{
		SessionID: start.SessionID, RunID: start.RunID, ItemID: start.ItemID,
		OccurredAt: start.OccurredAt,
	}).Validate(); err != nil {
		return err
	}
	if start.Kind != transcript.AgentMessage && start.Kind != transcript.Reasoning && start.Kind != transcript.ToolCall {
		return fmt.Errorf("runs: unsupported Item start kind %d", start.Kind)
	}
	if start.Kind != transcript.ToolCall {
		if start.ToolInvocation != nil || start.SafetyClass != "" || start.durable != nil {
			return errors.New("runs: transient Item start carries ToolCall facts")
		}
		return nil
	}
	if start.ToolInvocation == nil || strings.TrimSpace(start.ToolInvocation.Name) == "" || start.durable == nil {
		return errors.New("runs: ToolCall start has no durable invocation")
	}
	item := *start.durable
	if item.SessionID() != start.SessionID || item.RunID() != start.RunID ||
		item.ID() != start.ItemID || item.Kind() != start.Kind ||
		!item.OccurredAt().Equal(start.OccurredAt) {
		return errors.New("runs: ToolCall start differs from its durable Item")
	}
	invocation, present := item.ToolInvocation()
	if !present || invocation.Name != start.ToolInvocation.Name ||
		invocation.Arguments != start.ToolInvocation.Arguments ||
		item.SafetyClass() != start.SafetyClass {
		return errors.New("runs: ToolCall start differs from its durable invocation")
	}
	return item.Validate()
}

func (start ItemStart) durableItem() (transcript.Item, bool) {
	if start.durable == nil {
		return transcript.Item{}, false
	}
	return *start.durable, true
}
