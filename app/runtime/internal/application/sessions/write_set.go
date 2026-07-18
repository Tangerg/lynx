package sessions

import (
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type RollbackPlan struct {
	SessionID      string
	RunID          string
	KeepMark       int
	DropRunIDs     []string
	DropSessionIDs []string
	ProcessIDs     []string
	Terminate      bool
}

type ForkPlan struct {
	ParentID string
	Messages []chat.Message
	Title    string
}

type RestorePlan = Snapshot

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
