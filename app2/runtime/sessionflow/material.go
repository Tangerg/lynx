package sessionflow

import (
	"time"

	conversationdomain "github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	goaldomain "github.com/Tangerg/lynx/app2/runtime/domain/goal"
	plandomain "github.com/Tangerg/lynx/app2/runtime/domain/plan"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
	"github.com/Tangerg/lynx/app2/runtime/domain/toolresult"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

type Material struct {
	Session        session.Session
	Runs           []rundomain.Record
	Items          []transcript.Record
	Messages       []conversationdomain.Record
	Interrupts     []protocol.PendingInterruptSet
	Plan           plandomain.State
	PlanBoundaries map[string]plandomain.Boundary
	Goal           *goaldomain.Goal
	ToolResults    []toolresult.Record
}

type ForkWrite struct {
	Session        session.Session
	Runs           []rundomain.Record
	Items          []transcript.Record
	Messages       []conversationdomain.Record
	Plan           *plandomain.State
	PlanBoundaries map[string]plandomain.Boundary
	ToolResults    []toolresult.Record
}

type RollbackWrite struct {
	SessionID            session.ID
	DropRootRunIDs       []string
	Plan                 *plandomain.State
	ExpectedPlanRevision uint64
	Now                  time.Time
}

type ImportWrite struct {
	Material Material
}

type ForkResult struct {
	Session     *protocol.Session
	PlanChanged bool
}

type RollbackResult struct {
	Response       *protocol.RollbackSessionResponse
	PlanChanged    bool
	HistoryChanged bool
}

type ImportResult struct {
	Response    *protocol.ImportSessionResponse
	PlanChanged bool
}
