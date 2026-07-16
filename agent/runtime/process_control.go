package runtime

import (
	"context"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
)

type processControl struct{ process *Process }

var _ core.ProcessControl = processControl{}

func (c processControl) TerminateAgent(reason string) {
	c.process.signals.queueTermination(core.TerminationScopeAgent, reason)
}

func (c processControl) TerminateAction(reason string) {
	c.process.signals.queueTermination(core.TerminationScopeAction, reason)
}

func (c processControl) TerminateToolCall() {
	c.process.signals.fireToolCallCancel()
}

// Suspension returns a defensive copy of the durable continuation currently
// owned by this process.
func (p *Process) Suspension() *interaction.Suspension {
	if p == nil {
		return nil
	}
	return p.state.suspension()
}

func (c processControl) Suspend(ctx context.Context, suspension interaction.Suspension) (core.ActionStatus, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	process := c.process
	if err := process.validateNestedSuspension(suspension); err != nil {
		process.state.recordFailure(err)
		return core.ActionFailed, err
	}
	if err := process.state.parkSuspension(suspension); err != nil {
		process.state.recordFailure(err)
		return core.ActionFailed, err
	}
	process.commitNestedSuspension()
	process.publishEvent(ctx, event.ProcessWaiting{Header: process.eventHeader(), Suspension: process.Suspension()})
	return core.ActionWaiting, nil
}
