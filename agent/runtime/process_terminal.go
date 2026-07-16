package runtime

import (
	"context"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// validateAgentForRun checks the agent definition against the configured
// planner. The goap planner needs at least one goal to plan toward;
// with no goal the planner would loop forever returning empty plans.
// (htn, reactive) may have stricter rules of their own — those are
// reported by PlanToGoal at tick time.
func (p *Process) validateAgentForRun() error {
	agent := p.agent()
	if p.planner.Name() == "goap" && len(agent.Goals()) == 0 {
		return fmt.Errorf("runtime.Process.validateAgentForRun: run agent %q: goap planner requires at least one goal", agent.Name())
	}
	return nil
}

// failProcess transitions to StatusFailed and records the failure.
// It deliberately does NOT publish [event.ProcessFailed] — every run
// exit funnels through [publishTerminalEvent], which is the single
// publisher of terminal events (publishing here too double-fired
// ProcessFailed on planner errors).
func (p *Process) failProcess(err error) {
	p.state.recordFailure(err)
	p.state.transition(core.StatusFailed)
}

// markCancelled records context cancellation as a kill. Publishes ProcessKilled
// only if it won the terminal transition — an external Kill racing the
// ctx-cancel path must not double-publish (transition is the first-terminal-wins
// gate).
func (p *Process) markCancelled(ctx context.Context, err error) {
	p.state.recordFailure(err)
	if p.state.transition(core.StatusKilled) {
		p.publishEvent(ctx, event.ProcessKilled{
			Header: p.eventHeader(),
			Reason: err.Error(),
		})
	}
}

// checkStopPolicies asks every applicable [core.StopPolicy]
// — the implicit Budget-derived policy plus any policy extensions
// registered at engine or process scope — and terminates the
// process at the first "yes". Returns true when the run loop should
// exit.
func (p *Process) checkStopPolicies(ctx context.Context) bool {
	policies := append(
		[]core.StopPolicy{core.BudgetPolicy{Budget: p.options.Budget}},
		collectExtensions[core.StopPolicy](p.combinedExtensions())...,
	)
	for _, policy := range policies {
		stop, reason := policy.Check(p)
		if !stop {
			continue
		}
		if p.state.transition(core.StatusTerminated) {
			p.publishEvent(ctx, event.ProcessTerminated{
				Header: p.eventHeader(),
				Reason: reason,
			})
		}
		return true
	}
	return false
}

// publishTerminalEvent dispatches the terminal-state event matching the
// current status.
func (p *Process) publishTerminalEvent(ctx context.Context) {
	switch p.Status() {
	case core.StatusCompleted:
		result, _ := core.Last[any](p.Blackboard())
		p.publishEvent(ctx, event.ProcessCompleted{
			Header: p.eventHeader(),
			Goal:   p.Goal(),
			Result: result,
		})
	case core.StatusFailed:
		p.publishEvent(ctx, event.ProcessFailed{
			Header: p.eventHeader(),
			Err:    p.Failure(),
		})
	}
}

// completeForGoal flips the process to Completed and publishes the goal
// achievement event. A no-op when a racing kill already terminated the process
// — first terminal wins, so the run loop can't clobber a Killed back to
// Completed (which would also double-publish a terminal at the loop's exit).
func (p *Process) completeForGoal(ctx context.Context, goal *core.Goal) {
	if !p.state.transition(core.StatusCompleted) {
		return
	}
	p.state.pursue(goal)
	p.publishEvent(ctx, event.GoalAchieved{
		Header: p.eventHeader(),
		Goal:   goal,
	})
}

// translateActionStatus maps the per-action ActionStatus to the
// process-level status. ActionSucceeded is a no-op (the next tick keeps
// running).
func (p *Process) translateActionStatus(action core.Action, status core.ActionStatus) {
	switch status {
	case core.ActionSucceeded:
		// Stay running — the next tick re-plans.
	case core.ActionFailed:
		if p.state.transition(core.StatusFailed) {
			// Don't overwrite a failure already recorded by an earlier action
			// — the first failure is the root cause worth surfacing.
			if p.Failure() == nil {
				p.state.recordFailure(p.actionFailure(action.Metadata().Name))
			}
		}
	case core.ActionWaiting:
		p.state.transition(core.StatusWaiting)
	case core.ActionPaused:
		p.state.transition(core.StatusPaused)
	}
}

// handleStuck is invoked when the planner returned no plan. If the agent
// supplied a StuckPolicy that resolves the situation, re-loop;
// otherwise, transition to Stuck.
func (p *Process) handleStuck(ctx context.Context, worldState core.WorldState) error {
	if handler := p.agent().StuckPolicy(); handler != nil {
		if result := handler.Recover(ctx, p, p.blackboard); result.Decision == core.StuckReplan {
			p.state.clearExclusions()
			return nil
		}
	}

	if p.state.transition(core.StatusStuck) {
		p.publishEvent(ctx, event.ProcessStuck{
			Header: p.eventHeader(),
			State:  worldState,
		})
	}
	return nil
}
