package runtime

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
)

// run drives the OODA loop until the process terminates. Internal — the
// only caller is Platform.RunAgent / StartAgent, which Platform exposes.
func (p *AgentProcess) run(ctx context.Context) error {
	ctx = normalizeContext(ctx)

	// makeRunning is a CAS — if the process is already running (e.g.
	// a double StartAgent call), it returns false and the call silently no-ops.
	if !p.state.makeRunning() {
		return nil
	}

	if err := p.validateAgentForRun(); err != nil {
		p.failProcess(err)
		p.publishTerminalEvent()
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			p.markCancelled(err)
			return err
		}

		if p.checkEarlyTermination() {
			p.maybeAutoSnapshot(ctx)
			p.recordRunExitMetric(ctx)
			return nil
		}

		if err := p.tick(ctx); err != nil {
			return err
		}

		// Persist the post-tick state when auto-snapshot is on. Placed
		// after Tick so it captures whatever status the tick produced —
		// including the terminal / waiting one on the loop's last pass.
		p.maybeAutoSnapshot(ctx)

		// Keep ticking only while Running. Waiting / Paused / Stuck /
		// terminal all release the loop so the host (HITL resume,
		// stuck-handler, terminal cleanup) can drive next.
		if p.Status() != core.StatusRunning {
			p.publishTerminalEvent()
			p.recordRunExitMetric(ctx)
			return nil
		}
	}
}

// maybeAutoSnapshot persists the current process state when the platform
// has auto-snapshot enabled and a store configured. Best-effort: a
// persistence failure is recorded on a span but never aborts the running
// process — losing a snapshot is recoverable, killing a live agent is not.
func (p *AgentProcess) maybeAutoSnapshot(ctx context.Context) {
	if p.platform == nil || !p.platform.autoSnapshot || p.platform.processStore == nil {
		return
	}

	if err := p.platform.processStore.Save(ctx, p.Snapshot()); err != nil {
		_, span := core.AgentTracer().Start(ctx, "agent.auto_snapshot")
		span.SetAttributes(attribute.String(attrProcessID, p.id))
		finishSpanWithError(span, err)
		span.End()
	}
}

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
func (p *AgentProcess) markCancelled(err error) {
	p.state.setFailure(err)
	if p.state.setStatus(core.StatusKilled) {
		p.publishEvent(event.ProcessKilled{
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
func (p *AgentProcess) checkEarlyTermination() bool {
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
			p.publishEvent(event.ProcessTerminated{
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
func (p *AgentProcess) publishTerminalEvent() {
	switch p.Status() {
	case core.StatusCompleted:
		p.publishEvent(event.ProcessCompleted{
			BaseEvent: p.baseEvent(),
			Goal:      p.Goal(),
		})
	case core.StatusFailed:
		p.publishEvent(event.ProcessFailed{
			BaseEvent: p.baseEvent(),
			Err:       p.Failure(),
		})
	}
}

// Tracing attribute / span keys local to the tick loop.
const (
	spanTick           = "agent.tick"
	attrWorldStateSize = "agent.world_state.size"
)

// tick performs one OODA iteration — the run loop's single step. The
// loop in run drives it; production callers go through
// [Platform.RunAgent] / [Platform.StartAgent] / [Platform.ContinueProcess]
// which run the full loop.
func (p *AgentProcess) tick(ctx context.Context) error {
	ctx = normalizeContext(ctx)
	ctx = core.WithProcess(ctx, p)

	if signal := p.signals.drainTerminate(); signal != nil {
		return p.handleTerminationSignal(*signal)
	}

	ctx, span := p.startTickSpan(ctx, spanTick)
	defer span.End()
	p.recordTickMetric(ctx)

	worldState := p.observe(ctx, span)

	if p.options.ProcessType == core.ProcessConcurrent {
		return p.tickConcurrent(ctx, worldState)
	}
	return p.tickSimple(ctx, worldState)
}

// observe runs the determiner and publishes the ReadyToPlan event.
func (p *AgentProcess) observe(ctx context.Context, span trace.Span) core.WorldState {
	worldState := p.determiner.determineWorldState(ctx)
	p.state.setLastWorld(worldState)

	p.publishEvent(event.ReadyToPlan{
		BaseEvent: p.baseEvent(),
		World:     worldState,
	})
	span.SetAttributes(attribute.Int(attrWorldStateSize, len(worldState.State())))
	return worldState
}

// handleTerminationSignal processes a queued termination request. AGENT-
// scope signals stop the process; ACTION-scope signals trigger a re-plan
// without running an action this tick.
func (p *AgentProcess) handleTerminationSignal(sig core.TerminationSignal) error {
	switch sig.Scope {
	case core.TerminationScopeAgent:
		if p.state.setStatus(core.StatusTerminated) {
			p.publishEvent(event.ProcessTerminated{
				BaseEvent: p.baseEvent(),
				Reason:    sig.Reason,
				Scope:     core.TerminationScopeAgent,
			})
		}

	case core.TerminationScopeAction:
		p.publishEvent(event.ReplanRequested{
			BaseEvent: p.baseEvent(),
			Reason:    sig.Reason,
		})
	}
	return nil
}

// tickSimple runs the first applicable action of the best plan.
func (p *AgentProcess) tickSimple(ctx context.Context, worldState core.WorldState) error {
	planResult, done, err := p.planForTick(ctx, worldState)
	if err != nil || done {
		return err
	}

	action := planResult.Actions[0]
	status, replan := p.executeAction(ctx, action)
	if err := ctx.Err(); err != nil {
		p.markCancelled(err)
		return nil
	}
	if replan != nil {
		p.applyReplan(action, replan)
		return nil
	}

	p.translateActionStatus(action, status)
	return nil
}

// planForTick is the shared prelude both tickSimple and tickConcurrent
// run before they decide which action(s) to execute. It plans, handles
// the three "no action this tick" outcomes (planner error → fail,
// no plan → stuck, plan complete → goal achieved), and on success sets
// the process goal and publishes [event.PlanFormulated].
//
// Return shape:
//
//   - planResult, false, nil  — caller should proceed to execute the plan
//   - nil,        true,  nil  — caller should return immediately (process
//     transitioned via failProcess / handleStuck / completeForGoal)
//   - nil,        true,  err  — Tick should propagate err (handleStuck
//     can't currently produce one but the contract leaves room)
func (p *AgentProcess) planForTick(ctx context.Context, worldState core.WorldState) (*planning.Plan, bool, error) {
	planStart := core.Now()
	planResult, err := p.formulatePlan(ctx, worldState)
	p.recordPlanMetric(ctx, time.Since(planStart))
	if err != nil {
		p.failProcess(err)
		return nil, true, nil
	}
	if planResult == nil {
		return nil, true, p.handleStuck(ctx, worldState)
	}
	if planResult.IsComplete() {
		p.completeForGoal(planResult.Goal)
		return nil, true, nil
	}

	p.state.setGoal(planResult.Goal)
	p.publishEvent(event.PlanFormulated{
		BaseEvent: p.baseEvent(),
		Plan:      planResult,
	})
	return planResult, false, nil
}

// formulatePlan runs the configured planner against the current world
// state, honoring the running exclusion list. The planning.System is
// allocated once per process at createProcess time so its KnownConditions
// cache survives across ticks.
//
// Registered [core.GoalApprover] extensions filter the goal set before
// the planner sees it — an unanimous "yes" is required for a goal to
// remain plannable for this tick. With no approvers registered the
// fast path reuses the cached planning.System.
func (p *AgentProcess) formulatePlan(ctx context.Context, worldState core.WorldState) (*planning.Plan, error) {
	system := p.system

	approvers := collectExtensions[core.GoalApprover](p.combinedExtensions())
	if len(approvers) > 0 {
		var approved []*core.Goal
		for _, goal := range system.Goals {
			if runGoalApprovers(approvers, p, goal) {
				approved = append(approved, goal)
			}
		}
		if len(approved) != len(system.Goals) {
			system = planning.NewSystem(system.Actions, approved, system.Conditions)
		}
	}

	return planning.BestValuePlan(
		ctx, p.planner, worldState, system,
		planning.Options{ExcludedActions: p.state.snapshotExclusions()},
	)
}

// completeForGoal flips the process to Completed and publishes the goal
// achievement event. A no-op when a racing kill already terminated the process
// — first terminal wins, so the run loop can't clobber a Killed back to
// Completed (which would also double-publish a terminal at the loop's exit).
func (p *AgentProcess) completeForGoal(g *core.Goal) {
	if !p.state.setStatus(core.StatusCompleted) {
		return
	}
	p.state.setGoal(g)
	p.publishEvent(event.GoalAchieved{
		BaseEvent: p.baseEvent(),
		Goal:      g,
	})
}

// applyReplan applies an action's replan request: stage its blackboard
// update, exclude the action, publish the event. Status stays Running so
// the next tick reformulates the plan.
func (p *AgentProcess) applyReplan(action core.Action, request *core.ReplanRequest) {
	p.state.excludeAction(action.Metadata().Name)
	if request.Update != nil {
		request.Update(p.blackboard)
	}
	p.publishEvent(event.ReplanRequested{
		BaseEvent: p.baseEvent(),
		Action:    action.Metadata().Name,
		Reason:    request.Reason,
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
		p.publishEvent(event.ProcessStuck{
			BaseEvent: p.baseEvent(),
			LastWorld: worldState,
		})
	}
	return nil
}
