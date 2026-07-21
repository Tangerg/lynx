package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
)

// TerminalError formats a non-completed terminal status as an error. Waiting
// is also an error here; adapters that expose waiting as structured state must
// branch on [core.ProcessStatus] before calling TerminalError.
func (p *Process) TerminalError() error {
	if p == nil {
		return errors.New("process is nil")
	}
	status := p.Status()
	if status == core.StatusCompleted {
		return nil
	}
	if failure := p.Failure(); failure != nil {
		return fmt.Errorf("ended in %s: %w", status, failure)
	}
	return fmt.Errorf("ended in %s", status)
}

// failProcess transitions to StatusFailed and records the failure.
// It deliberately does NOT publish [event.ProcessFailed] — every run
// exit funnels through [publishTerminalEvent], which is the single
// publisher of terminal events (publishing here too double-fired
// ProcessFailed on planner errors).
func (p *Process) failProcess(err error) {
	p.state.fail(err)
}

// markCancelled records context cancellation as a kill. Publishes ProcessKilled
// only if it won the terminal transition — an external Kill racing the
// ctx-cancel path must not double-publish (transition is the first-terminal-wins
// gate).
func (p *Process) markCancelled(ctx context.Context, err error) {
	if won, _ := p.state.markKilled(err); won {
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
func (p *Process) checkStopPolicies(ctx context.Context) (bool, error) {
	policies := append(
		[]extensionCapability[core.StopPolicy]{{
			name:  core.BudgetPolicyName,
			value: core.BudgetPolicy{Budget: p.options.budget},
		}},
		collectExtensions[core.StopPolicy](p.combinedExtensions())...,
	)
	for _, policy := range policies {
		stop, reason, err := checkStopPolicy(policy.value, p, policy.name)
		if err != nil {
			return false, err
		}
		if !stop {
			continue
		}
		if p.state.transition(core.StatusTerminated) {
			p.publishEvent(ctx, event.ProcessTerminated{
				Header: p.eventHeader(),
				Reason: reason,
			})
		}
		return true, nil
	}
	return false, nil
}

func checkStopPolicy(policy core.StopPolicy, process core.ProcessView, name string) (stop bool, reason string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("stop policy %q panicked", name), recovered)
		}
	}()
	stop, reason = policy.Check(process)
	return stop, reason, nil
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
func (p *Process) translateActionStatus(action core.Action, status core.ActionStatus, actionErr error) {
	switch status {
	case core.ActionSucceeded:
		// Stay running — the next tick re-plans.
	case core.ActionFailed:
		if actionErr == nil {
			actionErr = p.actionFailure(action.Metadata().Name)
		}
		p.state.fail(actionErr)
	case core.ActionWaiting:
		p.state.transition(core.StatusWaiting)
	case core.ActionPaused:
		p.state.transition(core.StatusPaused)
	}
}

// handleStuck is invoked when the planner returned no plan. If the agent
// supplied a StuckPolicy that resolves the situation, re-loop;
// otherwise, transition to Stuck.
func (p *Process) handleStuck(ctx context.Context, worldState core.WorldState) {
	var reason string
	if handler := p.agent().StuckPolicy(); handler != nil {
		result, err := p.recoverStuck(ctx, handler)
		if err != nil {
			p.failProcess(err)
			return
		}
		if !result.Decision.Valid() {
			p.failProcess(fmt.Errorf("runtime.Process.handleStuck: policy returned invalid decision %s", result.Decision))
			return
		}
		reason = result.Reason
		if result.Decision == core.StuckReplan {
			if p.state.beginStuckReplan(worldState.Key()) {
				p.state.clearExclusions()
				return
			}
			if reason == "" {
				reason = "stuck policy requested replanning without changing world state"
			}
		}
	}

	if p.state.transition(core.StatusStuck) {
		p.publishEvent(ctx, event.ProcessStuck{
			Header: p.eventHeader(),
			State:  worldState,
			Reason: reason,
		})
	}
}

func (p *Process) recoverStuck(ctx context.Context, policy core.StuckPolicy) (result core.StuckResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = panicerr.New(fmt.Sprintf("runtime.Process.handleStuck: stuck policy %T panicked", policy), recovered)
		}
	}()
	return policy.Recover(ctx, p, p.blackboard), nil
}
