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

// ForkPlan is the durable write-set a session fork commits atomically: branch a
// child session off ParentID, seed its chat log with the resolved history prefix,
// and (optionally) title it — so a concurrent delete on the parent can't race a
// half-created child. The application resolves the history prefix; the adapter
// commits the branch and returns the created child.
type ForkPlan struct {
	ParentID string
	Messages []chat.Message
	Title    string
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
