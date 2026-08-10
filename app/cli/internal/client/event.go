package client

import "time"

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

// Event is one closed, presentation-oriented fact from a runtime adapter.
type Event interface{ clientEvent() }

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

func (RunStarted) clientEvent()     {}
func (RunResumed) clientEvent()     {}
func (BlockStarted) clientEvent()   {}
func (BlockDelta) clientEvent()     {}
func (BlockCompleted) clientEvent() {}
func (PlanChanged) clientEvent()    {}
func (RunInterrupted) clientEvent() {}
func (RunFinished) clientEvent()    {}
