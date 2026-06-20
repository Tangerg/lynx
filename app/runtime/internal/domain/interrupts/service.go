// Package interrupts is the registry of open HITL interrupts — the
// durable-resumable side of the R-model human-in-the-loop flow (API.md
// §6). When a run parks for approval the runtime records a [Pending]
// here; the client discovers them via runs.listOpenInterrupts and
// answers via runs.resume, which removes the entry.
//
// The [Store] interface is the pluggable seam: the runtime depends on
// it, not on a concrete backend. Lyra backs it with the SQLite store
// (internal/infra/storage/sqlite), so open interrupts survive a restart and
// runs.resume rebuilds the parked process from its ProcessStore
// snapshot (Pending.ProcessID) — same-process resume just drives the
// retained live process instead.
package interrupts

import (
	"context"
	"encoding/json"
	"time"
)

// Pending is one parked run awaiting a human decision. ParentRunID is
// the interrupted run the client passes to runs.resume; TurnID is the
// runtime's internal handle for the live process the resume drives;
// ProcessID is the agent-process snapshot key used to REBUILD that
// process after a restart (the live TurnID is gone then). Interrupts is
// the wire payload (one entry per pending awaitable), stored opaquely as
// JSON so this package stays free of a protocol-type dependency.
// DrainedTools is the backend-private half of the park: resume
// bookkeeping the client never sees.
type Pending struct {
	ParentRunID string
	SessionID   string
	TurnID      string
	ProcessID   string
	// Provider + Model are the parked run's per-run model selection (the
	// runs.start{providerId, model} pair). Persisted so a cross-restart
	// rehydrate rebuilds the SAME model client instead of silently dropping to
	// the platform default — both empty means the run used the default. The live
	// process holds its client in memory, so same-process resume ignores these.
	Provider     string
	Model        string
	Interrupts   json.RawMessage
	DrainedTools []DrainedTool
	CreatedAt    time.Time
}

// DrainedTool records one tool item that was still open when the run
// parked (e.g. an ask_user call that interrupted from inside its own
// execution). The continuation uses it to re-bind the re-fired tool to
// its ORIGINAL item id — by (Name, canonical Arguments) — instead of
// minting a duplicate toolCall item. This is backend bookkeeping, kept
// as a typed field here rather than smuggled into the wire interrupt
// payload.
type DrainedTool struct {
	ItemID string `json:"itemId"`
	Name   string `json:"name"`
	// Arguments is the raw argument JSON exactly as the tool received
	// it; consumers canonicalize when keying.
	Arguments string `json:"arguments"`
}

// Store is the open-interrupt registry. Implementations must be safe
// for concurrent use. The interface is the consumer-side abstraction
// (the runtime + RPC server depend on it); back it with the sqlite
// store (internal/infra/storage/sqlite) or any persistent implementation.
type Store interface {
	// Put records (or replaces) a pending interrupt keyed by ParentRunID.
	Put(ctx context.Context, p Pending) error

	// List returns the pending interrupts for sessionID, or all of them
	// when sessionID is empty.
	List(ctx context.Context, sessionID string) ([]Pending, error)

	// Get returns the pending interrupt for parentRunID, ok=false when
	// none is recorded (resolved / unknown).
	Get(ctx context.Context, parentRunID string) (Pending, bool, error)

	// Consume atomically returns AND removes the pending interrupt for
	// parentRunID (ok=false when none is recorded). It is the resume path's
	// claim: read-and-remove in one operation makes resuming idempotent — a
	// second, concurrent resume of the same interrupt finds nothing and backs
	// off, so a non-idempotent tool can't re-fire. Distinct from [Store.Delete]
	// (a fire-and-forget drop that needs no prior read), used by cancel /
	// rollback / session cleanup.
	Consume(ctx context.Context, parentRunID string) (Pending, bool, error)

	// Delete removes the entry for parentRunID. Abandoning (cancel) or
	// sweeping (rollback / session delete) a run calls this; absent entries
	// are a no-op.
	Delete(ctx context.Context, parentRunID string) error
}
