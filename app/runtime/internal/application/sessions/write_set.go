package sessions

import (
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
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
}

func restorePlan(snapshot Snapshot) RestorePlan {
	return RestorePlan{
		Session:     snapshot.Session,
		Messages:    snapshot.Messages,
		Items:       snapshot.Items,
		Runs:        snapshot.Runs,
		ToolResults: snapshot.ToolResults,
	}
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
