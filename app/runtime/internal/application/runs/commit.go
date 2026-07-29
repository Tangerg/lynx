package runs

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
)

type StateChange uint8

const (
	StateUnchanged StateChange = iota
	StateSuspend
	StateTerminalize
)

type EventCommit struct {
	RunID     string
	SessionID string
	State     StateChange
	Outcome   execution.Outcome
	Items     []transcript.Item
	Run       *transcript.Run
	GoalTurn  *goal.TurnRecord
}

func (c EventCommit) isEmpty() bool {
	return len(c.Items) == 0 &&
		c.Run == nil &&
		c.GoalTurn == nil &&
		c.State == StateUnchanged
}

// TreeBarrierCommit is the one durable write-set produced when any executor
// suspension stops a Run tree. Pending owns the complete continuation hand-off;
// Runs contains one StateSuspend commit for every active Run in deterministic
// postorder. No individual Run commit may write or consume the root-owned set.
type TreeBarrierCommit struct {
	Pending interrupts.Pending
	Runs    []EventCommit
}
