package execution

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// StateChange is the durable Run-state transition an [EventCommit] applies, if
// any (§8.3): a run event that parks the Run suspends it, one that ends it
// terminalizes it, and a projection-only event leaves the state unchanged. The
// transition commits in the SAME transaction as the event's projections, so the
// admission state and the transcript can never disagree after a crash.
type StateChange uint8

const (
	// StateUnchanged leaves the Run's admission state as-is — a projection-only
	// event (a completed item, the run.started record).
	StateUnchanged StateChange = iota
	// StateSuspend transitions the Run running→interrupted (it parked for HITL
	// resume), committed atomically with the open-interrupt record.
	StateSuspend
	// StateTerminalize transitions the Run to terminal with [EventCommit.Outcome],
	// committed atomically with the terminal transcript record.
	StateTerminalize
)

// EventCommit is one atomic durable commit for a run event (§8.1/§8.3/§8.4): the
// Run-state transition plus the strongly-consistent projections that must land
// with it — the open-interrupt record, a transcript item, a transcript run — all
// in ONE transaction. The application builds it as an engine- and wire-neutral
// value; a persistence adapter applies every set part atomically, enriching the
// fields only it can resolve (an interrupt's ProcessID from the live turn, a
// terminal run's message watermark) inside that transaction. Zero-value fields
// are skipped, so one shape serves a park (Interrupt + StateSuspend), a terminal
// (Run + StateTerminalize), and a plain projection (Item, StateUnchanged).
//
// It replaces the pre-rewrite split where the interrupt record, the transcript
// projections, and the run-state transition were three separate best-effort
// writes: a crash between them could leave a parked run with no admission mark,
// or a terminal transcript with a still-running admission row.
type EventCommit struct {
	// SessionID scopes the run-state transition — the durable admission row is
	// keyed by session (one non-terminal Run per session, §8.2).
	SessionID string
	State     StateChange
	// Outcome is the terminal reason, applied with StateTerminalize; ignored
	// otherwise.
	Outcome Outcome
	// Interrupt, when set, opens the run's resumable record (a park). Its
	// ProcessID is left empty for the adapter to resolve from the live turn.
	Interrupt *interrupts.Pending
	// Item and Run are transcript projections. A Run with a negative Mark asks
	// the adapter to resolve the terminal message watermark inside the commit.
	Item *transcript.Item
	Run  *transcript.Run
}
