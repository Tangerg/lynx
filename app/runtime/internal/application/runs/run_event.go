package runs

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
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

type SegmentStarted struct{ Run transcript.Run }
type SegmentProgressed struct{ Progress RunProgress }
type SegmentFinished struct{ Run transcript.Run }
type ItemStarted struct{ Item transcript.Item }
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

func (event SegmentStarted) retainedBytes() int  { return retainedRunBytes(event.Run) }
func (SegmentProgressed) retainedBytes() int     { return 0 }
func (event SegmentFinished) retainedBytes() int { return retainedRunBytes(event.Run) }
func (event ItemStarted) retainedBytes() int     { return retainedItemBytes(event.Item) }
func (ItemChanged) retainedBytes() int           { return 0 }
func (event ItemCompleted) retainedBytes() int   { return retainedItemBytes(event.Item) }
func (event StateSnapshot) retainedBytes() int   { return retainedStateSnapshotBytes(event) }

type RunProgress struct {
	Step          *int
	Usage         *transcript.Usage
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
