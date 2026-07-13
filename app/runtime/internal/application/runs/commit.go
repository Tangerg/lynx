package runs

import (
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
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
	Item      *transcript.Item
	Run       *transcript.Run
}
