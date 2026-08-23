package sessionflow

import (
	"time"

	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Material struct {
	Session session.Session
	Runs []rundomain.Record
	Items []transcript.Record
	Messages []conversationdomain.Record
	Interrupts []protocol.PendingInterruptSet
	Plan protocol.Plan
	Goal *protocol.Goal
	ToolResults []toolresult.Record
}

type ForkWrite struct {
	Session session.Session
	Runs []rundomain.Record
	Items []transcript.Record
	Messages []conversationdomain.Record
	Plan *protocol.Plan
	ToolResults []toolresult.Record
}

type RollbackWrite struct {
	SessionID session.ID
	DropRootRunIDs []string
	Plan *protocol.Plan
	Now time.Time
}

type ImportWrite struct { Material Material }
