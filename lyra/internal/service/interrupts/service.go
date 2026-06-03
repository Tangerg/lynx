// Package interrupts is the registry of open HITL interrupts — the
// durable-resumable side of the R-model human-in-the-loop flow (API.md
// §6). When a run parks for approval the runtime records a [Pending]
// here; the client discovers them via runs.listOpenInterrupts and
// answers via runs.resume, which removes the entry.
//
// The [Store] interface is the pluggable seam: the runtime depends on
// it, not on a concrete backend. The default is [NewInMemory] (correct
// for same-process resume, where the live agent process is retained).
// A persistent backend (SQLite / file) becomes meaningful once
// cross-restart resume — rebuilding the parked process from a
// ProcessStore snapshot — lands; until then a persisted-but-
// unrestorable interrupt would be a dangling entry resume could never
// honor (API.md §6.2), so it is deliberately NOT the default.
package interrupts

import (
	"context"
	"encoding/json"
	"time"
)

// Pending is one parked run awaiting a human decision. ParentRunID is
// the interrupted run the client passes to runs.resume; TurnID is the
// runtime's internal handle for the live (or restorable) process the
// resume drives. Interrupts is the wire payload (one entry per pending
// awaitable), stored opaquely as JSON so this package stays free of a
// protocol-type dependency.
type Pending struct {
	ParentRunID string
	SessionID   string
	TurnID      string
	Interrupts  json.RawMessage
	CreatedAt   time.Time
}

// Store is the open-interrupt registry. Implementations must be safe
// for concurrent use. The interface is the consumer-side abstraction
// (the runtime + RPC server depend on it); back it with [NewInMemory]
// or any persistent implementation.
type Store interface {
	// Put records (or replaces) a pending interrupt keyed by ParentRunID.
	Put(ctx context.Context, p Pending) error

	// List returns the pending interrupts for sessionID, or all of them
	// when sessionID is empty.
	List(ctx context.Context, sessionID string) ([]Pending, error)

	// Get returns the pending interrupt for parentRunID, ok=false when
	// none is recorded (resolved / unknown).
	Get(ctx context.Context, parentRunID string) (Pending, bool, error)

	// Delete removes the entry for parentRunID. Resolving (resume) or
	// abandoning (cancel) a run calls this; absent entries are a no-op.
	Delete(ctx context.Context, parentRunID string) error
}
