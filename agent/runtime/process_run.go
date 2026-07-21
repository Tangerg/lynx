package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/planning"
)

// run drives the OODA loop until the process terminates. Internal — the
// only caller is Engine.Run / Start, which Engine exposes.
func (p *Process) run(ctx context.Context) error {
	started, err := p.beginRun()
	if err != nil {
		return err
	}
	if !started {
		return nil
	}
	return p.runOwned(ctx)
}

func (p *Process) beginRun() (bool, error) {
	started, err := p.state.beginRun()
	if err != nil {
		return false, fmt.Errorf("runtime.Process.run %q: %w", p.ID(), err)
	}
	return started, nil
}

func (p *Process) runOwned(ctx context.Context) error {
	ctx = normalizeContext(ctx)
	defer p.state.endRun()
	runCtx, cancelRun := context.WithCancel(ctx)
	releaseRun := p.signals.registerRunCancel(cancelRun)
	defer func() {
		cancelRun()
		releaseRun()
	}()
	// Close the registration race with Engine.Kill: a kill may win after
	// beginRun but before registerRunCancel publishes this invocation's
	// cancel function. Re-checking the terminal state after publication
	// guarantees either Kill observes the cancel function or this side
	// cancels itself.
	if p.Status() == core.StatusKilled {
		cancelRun()
	}
	ctx = runCtx

	for {
		if err := ctx.Err(); err != nil {
			p.markCancelled(ctx, err)
			snapshotErr := p.maybeAutoSnapshot(ctx)
			p.publishTerminalEvent(ctx)
			p.recordRunExitMetric(ctx)
			return errors.Join(err, snapshotErr)
		}

		stopped, stopErr := p.checkStopPolicies(ctx)
		if stopErr != nil {
			p.failProcess(fmt.Errorf("runtime.Process.run: check stop policies: %w", stopErr))
			stopped = true
		}
		if stopped {
			if err := p.maybeAutoSnapshot(ctx); err != nil {
				p.publishTerminalEvent(ctx)
				p.recordRunExitMetric(ctx)
				return err
			}
			p.publishTerminalEvent(ctx)
			p.recordRunExitMetric(ctx)
			return nil
		}

		if err := p.tick(ctx); err != nil {
			return err
		}

		// Persist the post-tick state when auto-snapshot is on. Placed
		// after Tick so it captures whatever status the tick produced —
		// including the terminal / waiting one on the loop's last pass.
		if err := p.maybeAutoSnapshot(ctx); err != nil {
			p.publishTerminalEvent(ctx)
			p.recordRunExitMetric(ctx)
			return err
		}

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
// [Engine.Run] / [Engine.Start] / [Engine.Continue]
// which run the full loop.
func (p *Process) tick(ctx context.Context) error {
	p.checkpointMu.Lock()
	defer p.checkpointMu.Unlock()
	ctx = normalizeContext(ctx)
	ctx = core.WithProcessView(ctx, p)

	if signal := p.signals.drainTerminate(); signal != nil {
		return p.handleTerminationSignal(ctx, *signal)
	}

	ctx, span := p.startTickSpan(ctx, spanTick)
	defer span.End()
	p.recordTickMetric(ctx)

	worldState, err := p.observe(ctx, span)
	if err != nil {
		p.failProcess(err)
		return nil
	}

	return p.tickSimple(ctx, worldState)
}

// observe runs the state reader and publishes the PlanningStarted event.
func (p *Process) observe(ctx context.Context, span trace.Span) (core.WorldState, error) {
	worldState, err := p.stateReader.read(ctx)
	if err != nil {
		return nil, fmt.Errorf("runtime.Process.observe: %w", err)
	}
	p.state.observe(worldState)

	p.publishEvent(ctx, event.PlanningStarted{
		Header: p.eventHeader(),
		State:  worldState,
	})
	span.SetAttributes(attribute.Int(attrWorldStateSize, len(worldState.Conditions())))
	return worldState, nil
}

// handleTerminationSignal processes a queued termination request. AGENT-
// scope signals stop the process; ACTION-scope signals trigger a re-plan
// without running an action this tick.
func (p *Process) handleTerminationSignal(ctx context.Context, sig core.TerminationSignal) error {
	switch sig.Scope {
	case core.TerminationScopeAgent:
		if p.state.transition(core.StatusTerminated) {
			p.publishEvent(ctx, event.ProcessTerminated{
				Header: p.eventHeader(),
				Reason: sig.Reason,
				Scope:  core.TerminationScopeAgent,
			})
		}

	case core.TerminationScopeAction:
		p.publishEvent(ctx, event.ReplanRequested{
			Header: p.eventHeader(),
			Reason: sig.Reason,
		})
	}
	return nil
}

// tickSimple runs the first applicable action of the best plan.
func (p *Process) tickSimple(ctx context.Context, worldState core.WorldState) error {
	planResult, done, err := p.planForTick(ctx, worldState)
	if err != nil || done {
		return err
	}

	actions := planResult.Actions()
	action := actions[0]
	status, replan := p.executeAction(ctx, action)
	if err := ctx.Err(); err != nil {
		p.markCancelled(ctx, err)
		return nil
	}
	if replan != nil {
		p.state.clearRespondedSuspension()
		p.applyReplan(ctx, action, replan)
		return nil
	}

	p.translateActionStatus(action, status)
	if status != core.ActionWaiting {
		p.state.clearRespondedSuspension()
	}
	return nil
}

// planForTick is the shared prelude both tickSimple and tickConcurrent
// run before they decide which action(s) to execute. It plans, handles
// the three "no action this tick" outcomes (planner error → fail,
// no plan → stuck, plan complete → goal achieved), and on success sets
// the process goal and publishes [event.PlanCreated].
//
// Return shape:
//
//   - planResult, false, nil  — caller should proceed to execute the plan
//   - nil,        true,  nil  — caller should return immediately (process
//     transitioned via failProcess / handleStuck / completeForGoal)
//   - nil,        true,  err  — Tick should propagate err (handleStuck
//     can't currently produce one but the contract leaves room)
func (p *Process) planForTick(ctx context.Context, worldState core.WorldState) (*planning.Plan, bool, error) {
	planStart := time.Now()
	planResult, err := p.formulatePlan(ctx, worldState)
	p.recordPlanMetric(ctx, time.Since(planStart))
	if err != nil {
		p.failProcess(err)
		return nil, true, nil
	}
	if planResult == nil {
		return nil, true, p.handleStuck(ctx, worldState)
	}
	p.state.clearStuckReplan()
	if planResult.Complete() {
		p.completeForGoal(ctx, planResult.Goal())
		return nil, true, nil
	}

	p.state.pursue(planResult.Goal())
	p.publishEvent(ctx, event.PlanCreated{
		Header: p.eventHeader(),
		Plan:   planResult,
	})
	return planResult, false, nil
}

// formulatePlan runs the configured planner against the current world
// state, honoring the running exclusion list. The planning.Domain is
// allocated once per process at createProcess time so its KnownConditions
// cache survives across ticks.
//
// Registered [core.GoalApprover] extensions filter the goal set before
// the planner sees it — an unanimous "yes" is required for a goal to
// remain plannable for this tick. With no approvers registered the
// fast path reuses the cached planning.Domain.
func (p *Process) formulatePlan(ctx context.Context, worldState core.WorldState) (*planning.Plan, error) {
	domain := p.domain

	approvers := collectExtensions[core.GoalApprover](p.combinedExtensions())
	if len(approvers) > 0 {
		var approved []*core.Goal
		for _, goal := range domain.Goals() {
			ok, err := p.approvesGoal(approvers, goal)
			if err != nil {
				return nil, fmt.Errorf("runtime.Process.formulatePlan: approve goal %q: %w", goal.Name(), err)
			}
			if ok {
				approved = append(approved, goal)
			}
		}
		if len(approved) != len(domain.Goals()) {
			var err error
			domain, err = planning.NewDomain(domain.Actions(), approved, domain.Conditions())
			if err != nil {
				return nil, fmt.Errorf("runtime.Process.formulatePlan: approved domain: %w", err)
			}
		}
	}

	return domain.BestPlan(
		ctx, p.planner, worldState,
		planning.Options{ExcludedActions: p.state.snapshotExclusions()},
	)
}

// applyReplan applies an action's replan request: stage its blackboard
// update, exclude the action, publish the event. Status stays Running so
// the next tick reformulates the plan.
func (p *Process) applyReplan(ctx context.Context, action core.Action, request *core.ReplanRequest) {
	p.state.excludeAction(action.Metadata().Name)
	if request.Update != nil {
		request.Update(p.blackboard)
	}
	p.publishEvent(ctx, event.ReplanRequested{
		Header:     p.eventHeader(),
		ActionName: action.Metadata().Name,
		Reason:     request.Reason,
	})
}
