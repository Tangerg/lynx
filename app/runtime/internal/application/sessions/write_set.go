package sessions

import (
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// RollbackPlan is the atomic durable command for truncating a session back to a
// run boundary. A parked run among DropRunIDs needs no terminalization: dropping
// its record is also how it releases the session's admission slot.
type RollbackPlan struct {
	SessionID      string
	KeepMark       int
	DropRunIDs     []string
	DropSessionIDs []string
	ProcessIDs     []string
}

type ForkPlan struct {
	ParentID string
	Messages []chat.Message
	Title    string
}

// RestorePlan is the atomic durable command for replacing a session aggregate.
// It is intentionally distinct from Snapshot, the export read model: the
// explicit command makes the persistence boundary's destructive operation
// visible instead of silently accepting every snapshot-shaped value.
type RestorePlan struct {
	Session     session.Session
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []transcript.Run
	ToolResults []offload.ToolResultBlob
	// Todos is the restored task list's semantic value. It is REPLACED rather than
	// cleared-and-rewritten: the projection's revision must come out greater than
	// whatever the target session already published, and a delete would restart the
	// revision space at one — leaving a client that holds a higher revision ignoring
	// the imported value as stale.
	Todos []todo.Item
}

func restorePlan(snapshot Snapshot) RestorePlan {
	return RestorePlan(snapshot)
}

// DeletePlan is the post-order session set removed by one delete cascade. It
// contains the addressed session plus its owned internal-subtask descendants;
// user-created forks are independent and are not included.
type DeletePlan struct {
	SessionIDs []string
}

// TerminalPlan is the complete durable projection for ending a parked run by
// cancellation or executor-state loss. The run becomes terminal, its interrupt
// items become incomplete, and its open-interrupt/admission records are closed
// in the same transaction.
type TerminalPlan struct {
	Run       transcript.Run
	Items     []transcript.Item
	ProcessID string
}
