package core

import (
	"context"
	"errors"
	"fmt"
)

// ExecuteSafely runs a.Execute(ctx, pc) under a panic guard,
// recording any recovered panic on the context (inspect via
// [ProcessContext.LastError]). A panic forces [ActionFailed].
func (pc *ProcessContext) ExecuteSafely(ctx context.Context, a Action) (status ActionStatus) {
	if a == nil {
		pc.recordError(errors.New("agent.ProcessContext.ExecuteSafely: execute action: action is nil"))
		return ActionFailed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if r := recover(); r != nil {
			pc.recordPanic(r)
			status = ActionFailed
		}
	}()
	return a.Execute(ctx, pc)
}

// recordError stashes err for the runtime to detect [ReplanRequest].
func (pc *ProcessContext) recordError(err error) { pc.lastErr = err }

func (pc *ProcessContext) recordPanic(panicValue any) {
	err, ok := panicValue.(error)
	if !ok {
		err = fmt.Errorf("agent.ProcessContext.recordPanic: action panicked: %v", panicValue)
	}
	pc.recordError(err)
}

// LastError returns the last error recorded via recordError (or nil).
func (pc *ProcessContext) LastError() error { return pc.lastErr }

// ResetError clears the per-call error slot. The runtime calls this
// between retries.
func (pc *ProcessContext) ResetError() { pc.lastErr = nil }
