// Package interrupts is the registry of open HITL interrupts — the
// durable-resumable side of the R-model human-in-the-loop flow (API.md
// §6). When a run parks for approval the runtime records a [Pending]
// here; the client discovers them via runs.listOpenInterrupts and
// answers via runs.resume, which removes the entry.
//
// This package holds the durable record shapes (Pending, DrainedTool) — a plain
// CRUD-over-[Pending] model. The park/resume orchestration lives in
// application/runs, not here; persistence is a consumer concern
// (each consumer declares the narrow interrupt port it needs), backed by the
// SQLite store so open interrupts survive a restart and runs.resume rebuilds the
// parked process from its ProcessStore snapshot (Pending.ProcessID) —
// same-process resume just drives the retained live process instead.
package interrupts

import (
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// Pending is one parked run awaiting a human decision. RunID is the STABLE
// logical run id the client passes to runs.resume — the same id across the run's
// whole park/resume history, never a per-segment one; TurnID is the runtime's
// internal handle for the live process the resume drives; ProcessID is the
// agent-process snapshot key used to REBUILD that process after a restart (the
// live TurnID is gone then). Interrupts is the canonical typed set of pending
// suspensions. DrainedTools is the backend-private half of the
// park: resume bookkeeping the client never sees.
type Pending struct {
	RunID     string
	SessionID string
	TurnID    string
	ProcessID string
	// Provider + Model are the parked run's per-run model selection (the
	// runs.start{providerId, model} pair). Persisted so a cross-restart
	// rehydrate rebuilds the SAME model client instead of silently dropping to
	// the platform default — both empty means the run used the default. The live
	// process holds its client in memory, so same-process resume ignores these.
	Provider     string
	Model        string
	Interrupts   []transcript.Interrupt
	DrainedTools []DrainedTool
	// RunCreatedAt is the RUN's original start time, carried unchanged across
	// every resume. A resume continuation stamps it back onto the run's durable
	// transcript record so the run keeps its timeline position (rollback/fork
	// ordering + subagent grouping) instead of jumping to the resume time.
	RunCreatedAt time.Time
	// CreatedAt is when THIS interrupt was recorded (the park), for ordering
	// listOpenInterrupts.
	CreatedAt time.Time
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
