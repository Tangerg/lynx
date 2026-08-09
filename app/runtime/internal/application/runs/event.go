package runs

import "time"

// Event is the application-owned Run event carried by the [journal].
//
// RunID and SegmentID are the ENVELOPE: which run and segment produced this
// event. They are not the stream's scope — one root stream carries its whole
// child-Run tree, so a child's event rides it bearing the child's own IDs.
type Event struct {
	RunID     string
	SegmentID string
	// Sequence is this event's position in its stream, assigned by the [journal]
	// that publishes it. It is the replay comparison key: a subscription resumes
	// after a sequence, and nothing orders events by comparing [Event.Cursor] as
	// a string.
	Sequence uint64
	// Cursor is the opaque token a client stores and hands back to resume after
	// this event. It encodes the stream's process epoch and scope alongside
	// Sequence, so a cursor from another process or another segment is refused
	// rather than resolved against a stream that never issued it. The journal
	// assigns it together with Sequence, so the two cannot disagree.
	Cursor    string
	Timestamp time.Time
	Payload   RunEvent
}

// Replayable supplies the journal's retention and lossless queue policy.
func (e Event) Replayable() bool { return e.Payload != nil && e.Payload.Replayable() }

// Terminal supplies the run lifecycle's segment-completion boundary.
func (e Event) Terminal() bool { return e.Payload != nil && e.Payload.Terminal() }
