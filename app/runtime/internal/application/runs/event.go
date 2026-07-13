package runs

import "time"

// Event is the transport-neutral run event the [Journal] carries and the
// delivery layer maps to the wire. Payload is opaque to the application (the
// concrete wire event lives only in delivery); Seq is the monotonic cursor the
// Coordinator mints ([Coordinator.mintCursor]) — an opaque, fixed-width,
// lexically-ordered position, so the Journal's replay stays correct while the
// evt_ wire framing is applied in delivery (§11.2).
type Event struct {
	RunID     string
	SegmentID string
	Seq       string
	Timestamp time.Time
	IsDurable bool
	IsTerm    bool
	Payload   Projection
}

// Durable, Terminal, and Cursor satisfy the [Journal]'s [Streamable] interface.
func (e Event) Durable() bool  { return e.IsDurable }
func (e Event) Terminal() bool { return e.IsTerm }
func (e Event) Cursor() string { return e.Seq }
