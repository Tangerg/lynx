package core

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/interaction"
)

// Suspend parks one durable continuation on the current process. The flag is
// per action invocation and lets typed actions translate a handled suspension
// into ActionWaiting without writing their zero output.
func (pc *ProcessContext) Suspend(ctx context.Context, suspension interaction.Suspension) (ActionStatus, error) {
	if pc == nil || pc.control == nil {
		if pc != nil && pc.parallelBranch {
			return ActionFailed, ErrParallelBranchControl
		}
		return ActionFailed, errors.New("agent: process context has no lifecycle control")
	}
	status, err := pc.control.Suspend(contextOrBackground(ctx), suspension)
	if err != nil {
		return status, err
	}
	if status == ActionWaiting {
		pc.suspended = true
	}
	return status, nil
}

// TerminateAgent requests process termination at the next tick boundary.
func (pc *ProcessContext) TerminateAgent(reason string) error {
	if pc == nil || pc.control == nil {
		return ErrParallelBranchControl
	}
	pc.control.TerminateAgent(reason)
	return nil
}

// TerminateAction requests re-planning without terminating the process.
func (pc *ProcessContext) TerminateAction(reason string) error {
	if pc == nil || pc.control == nil {
		return ErrParallelBranchControl
	}
	pc.control.TerminateAction(reason)
	return nil
}

// TerminateToolCall cancels the process's registered in-flight tool call.
func (pc *ProcessContext) TerminateToolCall() error {
	if pc == nil || pc.control == nil {
		return ErrParallelBranchControl
	}
	pc.control.TerminateToolCall()
	return nil
}
