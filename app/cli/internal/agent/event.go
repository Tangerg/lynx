package agent

import (
	"bytes"
	"slices"
	"time"
)

// RunEvent is one projected runtime event. EventID is an opaque replay token
// scoped to the root StreamSegmentID; SegmentID identifies the producing run.
type RunEvent struct {
	EventID         string
	RunID           string
	SegmentID       string
	StreamSegmentID string
	At              time.Time
	Event           Event
}

// StreamSegment returns the root segment whose journal owns this event. A
// root-only producer may omit StreamSegmentID because its producer segment is
// also the stream segment; tree adapters set it explicitly for every event.
func (e RunEvent) StreamSegment() string {
	if e.StreamSegmentID != "" {
		return e.StreamSegmentID
	}
	return e.SegmentID
}

func (e RunEvent) Clone() RunEvent {
	e.Event = CloneEvent(e.Event)
	return e
}

// Equal reports whether two envelopes contain the same durable event fact.
// Timestamp equality is based on the instant, not time.Location pointer or a
// process-local monotonic reading that cannot survive persistence.
func (e RunEvent) Equal(other RunEvent) bool {
	return e.EventID == other.EventID && e.RunID == other.RunID && e.SegmentID == other.SegmentID &&
		e.StreamSegment() == other.StreamSegment() && e.At.Equal(other.At) && equalEvent(e.Event, other.Event)
}

type Event interface{ isEvent() }

// SegmentStarted is the authoritative opening fact of every initial or resumed
// run segment.
type SegmentStarted struct{ Run Run }

type BlockStarted struct{ Block Block }

type BlockDelta struct {
	BlockID string
	Text    string
	// ContentIndex identifies the assistant content block receiving Text. Nil
	// means block zero and is also the only valid shape for reasoning and tool
	// output deltas.
	ContentIndex *int
}

// ToolArgumentsDelta carries the provisional JSON text used to assemble a
// tool invocation. The completed tool block remains authoritative; preserving
// this preview lets streaming clients expose it without teaching the
// conversation aggregate how to repair partial JSON.
type ToolArgumentsDelta struct {
	BlockID string
	Text    string
}

// RunProgress is a non-replayable preview of a running segment. Usage is
// run-cumulative when present; ContextTokens is the current context-window
// occupancy and may decrease after compaction.
type RunProgress struct {
	Step          *int
	Usage         *Usage
	ContextTokens *int64
	Activity      string
}

// CustomEvent preserves an extension event without coupling the CLI domain to
// a vendor-specific payload type. PayloadJSON always contains one valid JSON
// value, including "null" when the runtime supplied no payload.
type CustomEvent struct {
	Name        string
	PayloadJSON []byte
}

type BlockCompleted struct{ Block Block }

type PlanChanged struct {
	Revision uint64
	Items    []PlanItem
}

// RunInterrupted closes the current segment and parks the stable logical run.
// Interactions is the complete pending set that must be answered atomically;
// Usage is the run-cumulative metering committed at that segment boundary.
type RunInterrupted struct {
	Interactions []Interaction
	Usage        Usage
}

// RunSuspended closes a member segment because another run in the same tree
// interrupted. It carries no duplicate interactions; the tree-level pending
// set is assembled from the member that raised them.
type RunSuspended struct{ Usage Usage }

type RunFinished struct {
	Outcome Outcome
	Usage   Usage
}

func (SegmentStarted) isEvent()     {}
func (BlockStarted) isEvent()       {}
func (BlockDelta) isEvent()         {}
func (ToolArgumentsDelta) isEvent() {}
func (RunProgress) isEvent()        {}
func (CustomEvent) isEvent()        {}
func (BlockCompleted) isEvent()     {}
func (PlanChanged) isEvent()        {}
func (RunInterrupted) isEvent()     {}
func (RunSuspended) isEvent()       {}
func (RunFinished) isEvent()        {}

// ReplayableEvent reports whether the underlying runtime retains this event in
// its segment journal. Deltas are deliberately ephemeral.
func ReplayableEvent(event Event) bool {
	switch event.(type) {
	case SegmentStarted, BlockStarted, BlockCompleted, PlanChanged, RunInterrupted, RunSuspended, RunFinished:
		return true
	default:
		return false
	}
}

func CloneEvent(event Event) Event {
	switch item := event.(type) {
	case SegmentStarted:
		item.Run = item.Run.Clone()
		return item
	case BlockStarted:
		item.Block = item.Block.Clone()
		return item
	case BlockDelta:
		if item.ContentIndex != nil {
			item.ContentIndex = new(*item.ContentIndex)
		}
		return item
	case ToolArgumentsDelta:
		return item
	case RunProgress:
		if item.Step != nil {
			item.Step = new(*item.Step)
		}
		if item.Usage != nil {
			usage := item.Usage.Clone()
			item.Usage = &usage
		}
		if item.ContextTokens != nil {
			item.ContextTokens = new(*item.ContextTokens)
		}
		return item
	case CustomEvent:
		item.PayloadJSON = bytes.Clone(item.PayloadJSON)
		return item
	case BlockCompleted:
		item.Block = item.Block.Clone()
		return item
	case PlanChanged:
		item.Items = slices.Clone(item.Items)
		return item
	case RunInterrupted:
		item.Interactions = CloneInteractions(item.Interactions)
		item.Usage = item.Usage.Clone()
		return item
	case RunSuspended:
		item.Usage = item.Usage.Clone()
		return item
	case RunFinished:
		item.Outcome = item.Outcome.Clone()
		item.Usage = item.Usage.Clone()
		return item
	default:
		return nil
	}
}

func equalEvent(left, right Event) bool {
	switch item := left.(type) {
	case SegmentStarted:
		other, ok := right.(SegmentStarted)
		return ok && item.Run.Equal(other.Run)
	case BlockStarted:
		other, ok := right.(BlockStarted)
		return ok && item.Block.Equal(other.Block)
	case BlockDelta:
		other, ok := right.(BlockDelta)
		return ok && item.BlockID == other.BlockID && item.Text == other.Text &&
			equalOptional(item.ContentIndex, other.ContentIndex)
	case ToolArgumentsDelta:
		other, ok := right.(ToolArgumentsDelta)
		return ok && item == other
	case RunProgress:
		other, ok := right.(RunProgress)
		return ok && equalOptional(item.Step, other.Step) && equalOptionalUsage(item.Usage, other.Usage) &&
			equalOptional(item.ContextTokens, other.ContextTokens) && item.Activity == other.Activity
	case CustomEvent:
		other, ok := right.(CustomEvent)
		return ok && item.Name == other.Name && bytes.Equal(item.PayloadJSON, other.PayloadJSON)
	case BlockCompleted:
		other, ok := right.(BlockCompleted)
		return ok && item.Block.Equal(other.Block)
	case PlanChanged:
		other, ok := right.(PlanChanged)
		return ok && item.Revision == other.Revision && slices.Equal(item.Items, other.Items)
	case RunInterrupted:
		other, ok := right.(RunInterrupted)
		return ok && item.Usage.Equal(other.Usage) && equalInteractions(item.Interactions, other.Interactions)
	case RunSuspended:
		other, ok := right.(RunSuspended)
		return ok && item.Usage.Equal(other.Usage)
	case RunFinished:
		other, ok := right.(RunFinished)
		return ok && item.Outcome.Equal(other.Outcome) && item.Usage.Equal(other.Usage)
	case nil:
		return right == nil
	default:
		return false
	}
}

func equalOptional[T comparable](left, right *T) bool {
	return (left == nil) == (right == nil) && (left == nil || *left == *right)
}

func equalOptionalUsage(left, right *Usage) bool {
	return (left == nil) == (right == nil) && (left == nil || left.Equal(*right))
}

func equalInteractions(left, right []Interaction) bool {
	return slices.EqualFunc(left, right, func(left, right Interaction) bool {
		switch item := left.(type) {
		case Approval:
			other, ok := right.(Approval)
			return ok && item.Equal(other)
		case Question:
			other, ok := right.(Question)
			return ok && item.Equal(other)
		case nil:
			return right == nil
		default:
			return false
		}
	})
}
