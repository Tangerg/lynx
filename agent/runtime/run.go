package runtime

import (
	"context"
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
		p.publishTerminalEvent(ctx)
		return err
	}

	for {
		if err := ctx.Err(); err != nil {
			p.markCancelled(ctx, err)
			return err
		}

		if p.checkEarlyTermination(ctx) {
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
			p.publishTerminalEvent(ctx)
			p.recordRunExitMetric(ctx)
			return nil
		}
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
		return p.handleTerminationSignal(ctx, *signal)
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

	p.publishEvent(ctx, event.ReadyToPlan{
		BaseEvent: p.baseEvent(),
		World:     worldState,
	})
	span.SetAttributes(attribute.Int(attrWorldStateSize, len(worldState.State())))
	return worldState
}

// handleTerminationSignal processes a queued termination request. AGENT-
// scope signals stop the process; ACTION-scope signals trigger a re-plan
// without running an action this tick.
func (p *AgentProcess) handleTerminationSignal(ctx context.Context, sig core.TerminationSignal) error {
	switch sig.Scope {
	case core.TerminationScopeAgent:
		if p.state.setStatus(core.StatusTerminated) {
			p.publishEvent(ctx, event.ProcessTerminated{
				BaseEvent: p.baseEvent(),
				Reason:    sig.Reason,
				Scope:     core.TerminationScopeAgent,
			})
		}

	case core.TerminationScopeAction:
		p.publishEvent(ctx, event.ReplanRequested{
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
		p.markCancelled(ctx, err)
		return nil
	}
	if replan != nil {
		p.applyReplan(ctx, action, replan)
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
		p.completeForGoal(ctx, planResult.Goal)
		return nil, true, nil
	}

	p.state.setGoal(planResult.Goal)
	p.publishEvent(ctx, event.PlanFormulated{
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

// applyReplan applies an action's replan request: stage its blackboard
// update, exclude the action, publish the event. Status stays Running so
// the next tick reformulates the plan.
func (p *AgentProcess) applyReplan(ctx context.Context, action core.Action, request *core.ReplanRequest) {
	p.state.excludeAction(action.Metadata().Name)
	if request.Update != nil {
		request.Update(p.blackboard)
	}
	p.publishEvent(ctx, event.ReplanRequested{
		BaseEvent: p.baseEvent(),
		Action:    action.Metadata().Name,
		Reason:    request.Reason,
	})
}
