package agent

import (
	"slices"
	"time"
)

// Cursor is a session-local event position. Zero means no event has been
// accepted yet; real events start at one.
type Cursor uint64

// Envelope gives an event durable replay identity. ID detects conflicting
// duplicates while Cursor orders events and makes reconnect resumable.
type Envelope struct {
	ID        string
	Cursor    Cursor
	RunID     string
	SessionID string
	At        time.Time
	Event     Event
}

// Clone returns an envelope whose event owns its mutable projections.
func (e Envelope) Clone() Envelope {
	e.Event = CloneEvent(e.Event)
	return e
}

// Event is one closed, presentation-oriented fact from a runtime adapter.
type Event interface{ isEvent() }

// RunStarted announces a logical run in the session timeline.
type RunStarted struct {
	RunID     string
	SessionID string
	Options   RunOptions
}

// RunResumed records that an interrupt answer was accepted.
type RunResumed struct{ InterruptID string }

// BlockStarted appends a block whose body may stream through BlockDelta.
type BlockStarted struct{ Block Block }

// BlockDelta appends text to a previously started streaming block. For tool
// blocks it appends output; for assistant and reasoning blocks it appends Text.
type BlockDelta struct {
	BlockID string
	Text    string
}

// BlockCompleted replaces a block with its authoritative final projection.
type BlockCompleted struct{ Block Block }

// PlanChanged carries the whole current plan.
type PlanChanged struct{ Items []PlanItem }

// RunInterrupted ends a subscription while leaving the logical run waiting.
type RunInterrupted struct{ Interaction Interaction }

// RunFinished ends the logical run.
type RunFinished struct {
	Outcome Outcome
	Usage   Usage
}

func (RunStarted) isEvent()     {}
func (RunResumed) isEvent()     {}
func (BlockStarted) isEvent()   {}
func (BlockDelta) isEvent()     {}
func (BlockCompleted) isEvent() {}
func (PlanChanged) isEvent()    {}
func (RunInterrupted) isEvent() {}
func (RunFinished) isEvent()    {}

// CloneEvent returns a detached copy of a known event payload.
func CloneEvent(event Event) Event {
	switch item := event.(type) {
	case RunStarted:
		return item
	case RunResumed:
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
		item.Interaction = CloneInteraction(item.Interaction)
		return item
	case RunFinished:
		return item
	default:
		return nil
	}
}
