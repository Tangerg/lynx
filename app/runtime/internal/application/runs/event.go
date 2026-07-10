package runs

import "time"

// Event is the transport-neutral run event the [Journal] carries and the
// delivery layer maps to the wire. Payload is opaque to the application (the
// concrete wire event lives only in delivery); Seq is the monotonic cursor
// minted by the delivery layer (see [CursorMinter]), which is what lets the
// Journal's lexical replay stay correct without knowing the cursor's format.
type Event struct {
	RunID     string
	Seq       string
	Timestamp time.Time
	IsDurable bool
	IsTerm    bool
	Payload   any
}

// Durable, Terminal, and Cursor satisfy the [Journal]'s [Streamable] interface.
func (e Event) Durable() bool  { return e.IsDurable }
func (e Event) Terminal() bool { return e.IsTerm }
func (e Event) Cursor() string { return e.Seq }
