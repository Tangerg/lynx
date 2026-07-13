package sessions

import (
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

type RollbackPlan struct {
	SessionID  string
	RunID      string
	KeepMark   int
	DropRunIDs []string
	Terminate  bool
}

type ForkPlan struct {
	ParentID string
	Messages []chat.Message
	Title    string
}

type RestorePlan struct {
	Session  session.Session
	Messages []chat.Message
	Runs     []transcript.Run
	Items    []transcript.Item
}
