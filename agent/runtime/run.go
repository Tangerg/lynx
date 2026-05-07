package runtime

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/plan"
)

// run drives the OODA loop until the process terminates. Internal — the
// only caller is Platform.RunAgent / StartAgent, which Platform exposes.
func (p *AgentProcess) run(ctx context.Context) error {
	if !p.state.makeRunning() {
		return nil
	}

	if err := p.validateAgentForRun(); err != nil {
		p.failProcess(err)
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			p.markCancelled(err)
			return err
		}

		if p.checkEarlyTermination() {
			return nil
		}

		if err := p.Tick(ctx); err != nil {
			return err
		}

		// Mirror embabel's AbstractAgentProcess.run loop: keep ticking
		// only while Running. Waiting / Paused / Stuck / terminal all
		// release the loop so the host (HITL resume, stuck-handler,
		// terminal cleanup) can drive next.
		if p.Status() != core.StatusRunning {
			p.publishTerminalEvent()
			return nil
		}
	}
}

// validateAgentForRun checks the agent definition against the configured
// planner. GOAP needs at least one goal to plan toward; without one we'd
// loop forever returning empty plans.
func (p *AgentProcess) validateAgentForRun() error {
	if p.options.PlannerType == core.PlannerGOAP && len(p.agent.Goals) == 0 {
		return fmt.Errorf("runtime.AgentProcess.run: agent %q has no goals (GOAP planner requires at least one)", p.agent.Name)
	}
	return nil
}

// failProcess transitions to StatusFailed and publishes the failure event.
func (p *AgentProcess) failProcess(err error) {
	p.state.setFailure(err)
	p.state.setStatus(core.StatusFailed)
	p.publishEvent(event.ProcessFailedEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Err:       err,
	})
}

// markCancelled records context cancellation as a kill.
func (p *AgentProcess) markCancelled(err error) {
	p.state.setFailure(err)
	p.state.setStatus(core.StatusKilled)
	p.publishEvent(event.ProcessKilledEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Reason:    err.Error(),
	})
}

// checkEarlyTermination consults the configured policy and, if it fires,
// flips the process to StatusTerminated. Returns true when the run loop
// should exit.
func (p *AgentProcess) checkEarlyTermination() bool {
	policy := p.options.ProcessControl.EarlyTerminationPolicy
	if policy == nil {
		return false
	}

	stop, reason := policy.ShouldTerminate(p)
	if !stop {
		return false
	}

	p.state.setStatus(core.StatusTerminated)
	p.publishEvent(event.ProcessTerminatedEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Reason:    reason,
	})
	return true
}

// publishTerminalEvent dispatches the terminal-state event matching the
// current status.
func (p *AgentProcess) publishTerminalEvent() {
	switch p.Status() {
	case core.StatusCompleted:
		p.publishEvent(event.ProcessCompletedEvent{
			BaseEvent: event.NewBaseEvent(p.id),
			Goal:      p.Goal(),
		})
	case core.StatusFailed:
		p.publishEvent(event.ProcessFailedEvent{
			BaseEvent: event.NewBaseEvent(p.id),
			Err:       p.Failure(),
		})
	case core.StatusStuck:
		p.publishEvent(event.ProcessStuckEvent{
			BaseEvent: event.NewBaseEvent(p.id),
			LastWorld: p.LastWorldState(),
		})
	}
}

// Tick performs one OODA iteration. Exported for tests that want to
// step frame-by-frame; production callers go through
// [Platform.RunAgent] / [Platform.StartAgent] / [Platform.ContinueProcess]
// which drive the full loop.
func (p *AgentProcess) Tick(ctx context.Context) error {
	ctx = core.WithProcess(ctx, p)

	if signal := p.signals.drainTerminate(); signal != nil {
		return p.handleTerminationSignal(*signal)
	}

	ctx, span := p.startTickSpan(ctx, spanTick)
	defer span.End()

	worldState := p.observe(ctx, span)

	if p.options.ProcessType == core.ProcessConcurrent {
		return p.tickConcurrent(ctx, worldState)
	}
	return p.tickSimple(ctx, worldState)
}

// observe runs the determiner and publishes the ReadyToPlan event.
func (p *AgentProcess) observe(ctx context.Context, span spanAttributer) core.WorldState {
	worldState := p.determiner.determineWorldState(ctx)
	p.state.setLastWorld(worldState)

	p.publishEvent(event.ReadyToPlanEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		World:     worldState,
	})
	span.SetAttributes(attribute.Int(attrWorldStateSize, len(worldState.State())))
	return worldState
}

// spanAttributer is the tiny subset of trace.Span observe needs — keeps
// the helper's signature decoupled from the full OTel span type so a
// future test stub doesn't need to implement everything.
type spanAttributer interface {
	SetAttributes(...attribute.KeyValue)
}

// handleTerminationSignal processes a queued termination request. AGENT-
// scope signals stop the process; ACTION-scope signals trigger a re-plan
// without running an action this tick.
func (p *AgentProcess) handleTerminationSignal(sig core.TerminationSignal) error {
	switch sig.Scope {
	case core.TerminationScopeAgent:
		p.state.setStatus(core.StatusTerminated)
		p.publishEvent(event.ProcessTerminatedEvent{
			BaseEvent: event.NewBaseEvent(p.id),
			Reason:    sig.Reason,
			Scope:     core.TerminationScopeAgent,
		})

	case core.TerminationScopeAction:
		p.publishEvent(event.ReplanRequestedEvent{
			BaseEvent: event.NewBaseEvent(p.id),
			Reason:    sig.Reason,
		})
	}
	return nil
}

// tickSimple runs the first applicable action of the best plan.
func (p *AgentProcess) tickSimple(ctx context.Context, ws core.WorldState) error {
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

	p.state.setGoal(planResult.Goal)
	p.publishEvent(event.PlanFormulatedEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Plan:      planResult,
	})

	action := planResult.Actions[0]
	status, replan := p.executeAction(ctx, action)
	if replan != nil {
		p.applyReplan(action, replan)
		return nil
	}

	p.translateActionStatus(action, status)
	return nil
}

// formulatePlan runs the configured planner against the current world
// state, honoring the running exclusion list. The PlanningSystem is
// allocated once per process at createProcess time so its KnownConditions
// cache survives across ticks.
func (p *AgentProcess) formulatePlan(ctx context.Context, ws core.WorldState) (*plan.Plan, error) {
	return p.planner.BestValuePlan(
		ctx, ws, p.system,
		plan.PlanOptions{ExcludedActions: p.state.snapshotExclusions()},
	)
}

// completeForGoal flips the process to Completed and publishes the goal
// achievement event.
func (p *AgentProcess) completeForGoal(g *core.Goal) {
	p.state.setStatus(core.StatusCompleted)
	p.state.setGoal(g)
	p.publishEvent(event.GoalAchievedEvent{
		BaseEvent: event.NewBaseEvent(p.id),
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
	p.publishEvent(event.ReplanRequestedEvent{
		BaseEvent: event.NewBaseEvent(p.id),
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
		p.state.setStatus(core.StatusFailed)
		if p.Failure() == nil {
			p.state.setFailure(actionFailureError(action.Metadata().Name))
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
	return fmt.Errorf("runtime.executeAction: action %q failed without recording an error", name)
}

// handleStuck is invoked when the planner returned no plan. If the agent
// supplied a StuckHandler that resolves the situation we re-loop;
// otherwise we transition to Stuck.
func (p *AgentProcess) handleStuck(ctx context.Context, ws core.WorldState) error {
	if handler := p.agent.StuckHandler; handler != nil {
		if result := handler.HandleStuck(ctx, p); result.Code == core.StuckReplan {
			p.state.clearExclusions()
			return nil
		}
	}

	p.state.setStatus(core.StatusStuck)
	p.publishEvent(event.ProcessStuckEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		LastWorld: ws,
	})
	return nil
}
