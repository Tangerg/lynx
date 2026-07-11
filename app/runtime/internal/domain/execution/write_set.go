package execution

import (
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// RollbackPlan is the durable write-set a rollback commits atomically (§8.1): the
// chat-log truncation to the boundary watermark, the per-run transcript record +
// open-interrupt drop for every run past the boundary, and — when the boundary
// abandons a parked run — the terminalization of that run's admission row. The
// application decides the plan (the boundary math); a persistence adapter applies
// every set part in ONE transaction. It replaces the pre-rewrite sequence of
// separate store writes joined by an implicit context transaction (§8.4).
type RollbackPlan struct {
	SessionID string
	// KeepMark truncates the chat log to this many messages; a negative mark
	// (unknown watermark — chain terminal in flight) leaves the log untouched.
	KeepMark int
	// DropRunIDs are the runs past the boundary: each run's transcript record and
	// any open interrupt are dropped.
	DropRunIDs []string
	// Terminate frees the session's durable admission slot — set when a dropped run
	// was parked (non-terminal), so the session can start a fresh run afterward.
	Terminate bool
}

// RestorePlan is the durable write-set a session restore/import commits
// atomically: recreate the session under its original id and REPLACE its whole
// history — clear the old open interrupts, admission rows, transcript, and chat
// log, then seed the decoded messages, runs, and items. Without one transaction a
// mid-sequence failure would leave the session row live but its history
// half-destroyed. The caller decodes the wire artifact into these domain values.
type RestorePlan struct {
	Session  session.Session
	Messages []chat.Message
	Runs     []transcript.Run
	Items    []transcript.Item
}
