package runtime

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// tickConcurrent runs every applicable action of the best plan in parallel.
// "Applicable" here means "preconditions are satisfied at the start of the
// tick" — actions whose inputs depend on a sibling's output stay sequential.
func (p *AgentProcess) tickConcurrent(ctx context.Context, ws core.WorldState) error {
	planResult, err := p.formulatePlan(ctx, ws)
	if err != nil {
		p.failProcess(err)
		return nil
	}
	if planResult == nil {
		return p.handleStuck(ctx, ws)
	}
	if planResult.IsComplete() {
		p.completeForGoal(planResult.Goal)
		return nil
	}

	p.setGoal(planResult.Goal)
	p.publishEvent(event.PlanFormulatedEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Plan:      planResult,
	})

	achievable := filterAchievable(planResult.Actions, ws)
	if len(achievable) == 0 {
		// Plan exists but nothing is currently runnable — fall back to
		// sequential mode for this tick (let the planner pick the best
		// candidate next iteration).
		return p.tickSimple(ctx, ws)
	}

	results, replans := p.runActionsInParallel(ctx, achievable)

	if p.applyReplansFromParallel(achievable, replans) {
		return nil
	}

	p.setStatus(mergeStatuses(results))
	return nil
}

// runActionsInParallel dispatches every achievable action onto its own
// goroutine and waits for completion. Result indices align with the input
// slice so the caller can correlate per-action outcomes.
func (p *AgentProcess) runActionsInParallel(ctx context.Context, actions []core.Action) ([]core.ActionStatus, []*core.ReplanRequest) {
	results := make([]core.ActionStatus, len(actions))
	replans := make([]*core.ReplanRequest, len(actions))

	var slotMu sync.Mutex
	g, egCtx := errgroup.WithContext(ctx)
	for index, action := range actions {
		index, action := index, action
		g.Go(func() error {
			status, replan := p.executeAction(egCtx, action)

			slotMu.Lock()
			results[index] = status
			replans[index] = replan
			slotMu.Unlock()
			return nil
		})
	}
	_ = g.Wait()

	return results, replans
}

// applyReplansFromParallel processes any replan requests returned by the
// parallel actions. Returns true when at least one was applied (caller
// should keep the process Running and re-plan next tick).
func (p *AgentProcess) applyReplansFromParallel(actions []core.Action, replans []*core.ReplanRequest) bool {
	hasReplan := false
	for index, replan := range replans {
		if replan == nil {
			continue
		}

		hasReplan = true
		p.applyReplan(actions[index], replan)
	}
	return hasReplan
}

// filterAchievable keeps only actions whose preconditions hold under the
// supplied world state. Order is preserved so the concurrent runner can
// correlate result indices with the plan's ordering.
func filterAchievable(actions []core.Action, ws core.WorldState) []core.Action {
	state := ws.State()
	out := make([]core.Action, 0, len(actions))
	for _, action := range actions {
		if isApplicableInState(action, state) {
			out = append(out, action)
		}
	}
	return out
}

func isApplicableInState(action core.Action, state map[string]core.Determination) bool {
	for key, required := range action.Metadata().Preconditions {
		if state[key] != required {
			return false
		}
	}
	return true
}

// mergeStatuses collapses a parallel result vector into one process status:
// failed/waiting/paused dominate; otherwise we keep running.
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
