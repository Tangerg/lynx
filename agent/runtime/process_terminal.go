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
func (p *AgentProcess) validateAgentForRun() error {
	if p.planner.Name() == "goap" && len(p.agent.Goals) == 0 {
		return fmt.Errorf("runtime.AgentProcess.validateAgentForRun: run agent %q: goap planner requires at least one goal", p.agent.Name)
	}
	return nil
}

// failProcess transitions to StatusFailed and records the failure.
// It deliberately does NOT publish [event.ProcessFailed] — every run
// exit funnels through [publishTerminalEvent], which is the single
// publisher of terminal events (publishing here too double-fired
// ProcessFailed on planner errors).
func (p *AgentProcess) failProcess(err error) {
	p.state.setFailure(err)
	p.state.setStatus(core.StatusFailed)
}

// markCancelled records context cancellation as a kill. Publishes ProcessKilled
// only if it won the terminal transition — an external KillProcess racing the
// ctx-cancel path must not double-publish (setStatus is the first-terminal-wins
// gate).
func (p *AgentProcess) markCancelled(ctx context.Context, err error) {
	p.state.setFailure(err)
	if p.state.setStatus(core.StatusKilled) {
		p.publishEvent(ctx, event.ProcessKilled{
			BaseEvent: p.baseEvent(),
			Reason:    err.Error(),
		})
	}
}

// checkEarlyTermination asks every applicable [core.EarlyTerminationPolicy]
// — the implicit Budget-derived policy plus any policy extensions
// registered at platform or process scope — and terminates the
// process at the first "yes". Returns true when the run loop should
// exit.
func (p *AgentProcess) checkEarlyTermination(ctx context.Context) bool {
	policies := append(
		[]core.EarlyTerminationPolicy{core.BudgetPolicy{Budget: p.options.Budget}},
		collectExtensions[core.EarlyTerminationPolicy](p.combinedExtensions())...,
	)
	for _, policy := range policies {
		stop, reason := policy.ShouldTerminate(p)
		if !stop {
			continue
		}
		if p.state.setStatus(core.StatusTerminated) {
			p.publishEvent(ctx, event.ProcessTerminated{
				BaseEvent: p.baseEvent(),
				Reason:    reason,
			})
		}
		return true
	}
	return false
}

// publishTerminalEvent dispatches the terminal-state event matching the
// current status.
func (p *AgentProcess) publishTerminalEvent(ctx context.Context) {
	switch p.Status() {
	case core.StatusCompleted:
		result, _ := core.Last[any](p.Blackboard())
		p.publishEvent(ctx, event.ProcessCompleted{
			BaseEvent: p.baseEvent(),
			Goal:      p.Goal(),
			Result:    result,
		})
	case core.StatusFailed:
		p.publishEvent(ctx, event.ProcessFailed{
			BaseEvent: p.baseEvent(),
			Err:       p.Failure(),
		})
	}
}

// completeForGoal flips the process to Completed and publishes the goal
// achievement event. A no-op when a racing kill already terminated the process
// — first terminal wins, so the run loop can't clobber a Killed back to
// Completed (which would also double-publish a terminal at the loop's exit).
func (p *AgentProcess) completeForGoal(ctx context.Context, g *core.Goal) {
	if !p.state.setStatus(core.StatusCompleted) {
		return
	}
	p.state.setGoal(g)
	p.publishEvent(ctx, event.GoalAchieved{
		BaseEvent: p.baseEvent(),
		Goal:      g,
	})
}

// translateActionStatus maps the per-action ActionStatus to the
// process-level status. ActionSucceeded is a no-op (the next tick keeps
// running).
func (p *AgentProcess) translateActionStatus(action core.Action, status core.ActionStatus) {
	switch status {
	case core.ActionSucceeded:
		// Stay running — the next tick re-plans.
	case core.ActionFailed:
		if p.state.setStatus(core.StatusFailed) {
			// Don't overwrite a failure already recorded by an earlier action
			// — the first failure is the root cause worth surfacing.
			if p.Failure() == nil {
				p.state.setFailure(actionFailureError(action.Metadata().Name))
			}
		}
	case core.ActionWaiting:
		p.state.setStatus(core.StatusWaiting)
	case core.ActionPaused:
		p.state.setStatus(core.StatusPaused)
	}
}

// actionFailureError produces a default failure error when the action
// returned ActionFailed without recording an explicit error on the
// ProcessContext (rare, but possible).
func actionFailureError(name string) error {
	return fmt.Errorf("runtime.actionFailureError: action %q failed without recording an error", name)
}

// handleStuck is invoked when the planner returned no plan. If the agent
// supplied a StuckPolicy that resolves the situation, re-loop;
// otherwise, transition to Stuck.
func (p *AgentProcess) handleStuck(ctx context.Context, worldState core.WorldState) error {
	if handler := p.agent.StuckPolicy; handler != nil {
		if result := handler.Recover(ctx, p); result.Code == core.StuckReplan {
			p.state.clearExclusions()
			return nil
		}
	}

	if p.state.setStatus(core.StatusStuck) {
		p.publishEvent(ctx, event.ProcessStuck{
			BaseEvent: p.baseEvent(),
			LastWorld: worldState,
		})
	}
	return nil
}
