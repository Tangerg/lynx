package agent

import (
	"slices"
	"time"
)

// RunEvent is one projected runtime event. EventID is an opaque replay token
// scoped to SegmentID.
type RunEvent struct {
	EventID   string
	RunID     string
	SegmentID string
	At        time.Time
	Event     Event
}

func (e RunEvent) Clone() RunEvent {
	e.Event = CloneEvent(e.Event)
	return e
}

type Event interface{ isEvent() }

// SegmentStarted is the authoritative opening fact of every initial or resumed
// run segment.
type SegmentStarted struct{ Run Run }

type BlockStarted struct{ Block Block }

type BlockDelta struct {
	BlockID string
	Text    string
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

type RunFinished struct {
	Outcome Outcome
	Usage   Usage
}

func (SegmentStarted) isEvent() {}
func (BlockStarted) isEvent()   {}
func (BlockDelta) isEvent()     {}
func (BlockCompleted) isEvent() {}
func (PlanChanged) isEvent()    {}
func (RunInterrupted) isEvent() {}
func (RunFinished) isEvent()    {}

// ReplayableEvent reports whether the underlying runtime retains this event in
// its segment journal. Deltas are deliberately ephemeral.
func ReplayableEvent(event Event) bool {
	switch event.(type) {
	case SegmentStarted, BlockStarted, BlockCompleted, PlanChanged, RunInterrupted, RunFinished:
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
	case RunFinished:
		item.Usage = item.Usage.Clone()
		return item
	default:
		return nil
	}
}
