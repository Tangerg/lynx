package runs

import "time"

// Event is the transport-neutral run event the [Journal] carries and the
// delivery layer maps to the wire. Seq is the monotonic cursor the
// Coordinator mints ([Coordinator.mintCursor]) — an opaque, fixed-width,
// lexically-ordered position, so the Journal's replay stays correct while the
// evt_ wire framing is applied in delivery (§11.2).
type Event struct {
	RunID     string
	SegmentID string
	Seq       string
	Timestamp time.Time
	Payload   RunEvent
}

// Durable, Terminal, and Cursor supply the Journal's replay and queue policy.
func (e Event) Durable() bool  { return e.Payload != nil && e.Payload.Durable() }
func (e Event) Terminal() bool { return e.Payload != nil && e.Payload.Terminal() }
func (e Event) Cursor() string { return e.Seq }
