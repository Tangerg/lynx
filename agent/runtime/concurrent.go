package runtime

import (
	"context"
	"sync"

	"github.com/Tangerg/lynx/agent/core"
)

// tickConcurrent runs every applicable action of the best plan in parallel.
// "Applicable" here means "preconditions are satisfied at the start of the
// tick" — actions whose inputs depend on a sibling's output stay sequential.
func (p *AgentProcess) tickConcurrent(ctx context.Context, worldState core.WorldState) error {
	planResult, done, err := p.planForTick(ctx, worldState)
	if err != nil || done {
		return err
	}

	achievable := filterAchievable(planResult.Actions, worldState)
	if len(achievable) == 0 {
		// Plan exists but nothing is currently runnable — fall back to
		// sequential mode for this tick (let the planner pick the best
		// candidate next iteration).
		return p.tickSimple(ctx, worldState)
	}

	results, replans := p.runActionsInParallel(ctx, achievable)
	if err := ctx.Err(); err != nil {
		p.markCancelled(ctx, err)
		return nil
	}

	if p.applyReplansFromParallel(ctx, achievable, replans) {
		return nil
	}

	p.state.setStatus(mergeStatuses(results))
	return nil
}

// runActionsInParallel dispatches every achievable action onto its own
// goroutine and waits for completion. Result indices align with the input
// slice so the caller can correlate per-action outcomes. Each goroutine
// writes a unique pre-allocated slot, and Wait synchronizes the writes
// with the post-Wait reads — so no explicit mutex is required.
//
// We use plain [sync.WaitGroup] (rather than [errgroup.Group]) because
// per-action failure is captured in the structured replans / results
// slices, not bubbled as an error — there's nothing for errgroup to
// fan in. Cancellation is honored by [executeAction] via the supplied
// ctx, so errgroup's auto-cancel-on-error is unnecessary.
func (p *AgentProcess) runActionsInParallel(ctx context.Context, actions []core.Action) ([]core.ActionStatus, []*core.ReplanRequest) {
	results := make([]core.ActionStatus, len(actions))
	replans := make([]*core.ReplanRequest, len(actions))

	var wg sync.WaitGroup
	for index, action := range actions {
		wg.Go(func() {
			results[index], replans[index] = p.executeAction(ctx, action)
		})
	}
	wg.Wait()

	return results, replans
}

// applyReplansFromParallel processes any replan requests returned by the
// parallel actions. Returns true when at least one was applied (caller
// should keep the process Running and re-plan next tick).
func (p *AgentProcess) applyReplansFromParallel(ctx context.Context, actions []core.Action, replans []*core.ReplanRequest) bool {
	hasReplan := false
	for index, replan := range replans {
		if replan == nil {
			continue
		}

		hasReplan = true
		p.applyReplan(ctx, actions[index], replan)
	}
	return hasReplan
}

// filterAchievable keeps only actions whose preconditions hold under the
// supplied world state. Order is preserved so the concurrent runner can
// correlate result indices with the plan's ordering.
func filterAchievable(actions []core.Action, worldState core.WorldState) []core.Action {
	state := worldState.State()
	out := make([]core.Action, 0, len(actions))
	for _, action := range actions {
		if action.Metadata().IsApplicableIn(state) {
			out = append(out, action)
		}
	}
	return out
}

// mergeStatuses collapses a parallel result vector into one process status:
// failed/waiting/paused dominate; otherwise the process keeps running.
func mergeStatuses(statuses []core.ActionStatus) core.AgentProcessStatus {
	for _, s := range statuses {
		if s == core.ActionFailed {
			return core.StatusFailed
		}
	}
	for _, s := range statuses {
		if s == core.ActionWaiting {
			return core.StatusWaiting
		}
	}
	for _, s := range statuses {
		if s == core.ActionPaused {
			return core.StatusPaused
		}
	}
	return core.StatusRunning
}
