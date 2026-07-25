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
	Interrupt *interrupts.Pending
	Items     []transcript.Item
	Run       *transcript.Run
	GoalTurn  *goal.TurnRecord
}

func (c EventCommit) isEmpty() bool {
	return len(c.Items) == 0 &&
		c.Run == nil &&
		c.Interrupt == nil &&
		c.GoalTurn == nil &&
		c.State == StateUnchanged
}
