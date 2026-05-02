package runtime

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// executeAction runs a single Action with retry, panic recovery, and
// post-action bookkeeping (history record, hasRun condition, events). It
// returns the final ActionStatus plus an optional ReplanRequest the action
// raised.
//
// The retry loop respects ActionQos: ActionFailed retries up to MaxAttempts
// with back-off; ActionWaiting/Paused/Succeeded short-circuit immediately.
func (p *AgentProcess) executeAction(ctx context.Context, action core.Action) (core.ActionStatus, *core.ReplanRequest) {
	meta := action.Metadata()
	startedAt := core.Now()

	p.publishEvent(event.ActionExecutionStartEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Action:    action,
		StartedAt: startedAt,
	})

	ctx, span := core.AgentTracer().Start(ctx, "lynx.agent.action")
	span.SetAttributes(
		attribute.String("lynx.agent.action.name", meta.Name),
		attribute.String("lynx.agent.process_id", p.id),
	)
	defer span.End()

	pc := p.buildProcessContext()

	status, replan, attempts, lastErr := p.runWithRetry(ctx, action, pc, meta.QoS)
	duration := time.Since(startedAt)

	p.recordInvocation(ActionInvocation{
		ActionName: meta.Name,
		Timestamp:  startedAt,
		Duration:   duration,
		Status:     status,
		Attempts:   attempts,
	})

	if status == core.ActionSucceeded {
		// hasRun gates non-rerunnable actions; we set it only on success so
		// retrying after a future re-plan remains possible.
		p.blackboard.SetCondition(core.HasRunKey(meta.Name), true)
	}

	span.SetAttributes(
		attribute.String("lynx.agent.action.status", status.String()),
		attribute.Int("lynx.agent.action.attempts", attempts),
	)
	finishSpanWithError(span, lastErr)

	p.publishEvent(event.ActionExecutionResultEvent{
		BaseEvent: event.NewBaseEvent(p.id),
		Action:    action,
		Status:    status,
		Duration:  duration,
		Err:       lastErr,
	})

	if status == core.ActionFailed && replan == nil {
		p.recordActionFailure(meta.Name, lastErr)
	}

	return status, replan
}

// runWithRetry implements the retry+back-off loop. It returns the final
// status, any replan request the action raised, the number of attempts
// made, and the most recent error (for span recording).
func (p *AgentProcess) runWithRetry(
	ctx context.Context,
	action core.Action,
	pc *core.ProcessContext,
	qos core.ActionQos,
) (status core.ActionStatus, replan *core.ReplanRequest, attempts int, lastErr error) {
	maxAttempts := qos.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	for attempts = 1; attempts <= maxAttempts; attempts++ {
		pc.ResetError()

		status = runWithPanicRecovery(ctx, action, pc)
		lastErr = pc.LastError()

		if rr := core.AsReplanRequest(lastErr); rr != nil {
			return status, rr, attempts, lastErr
		}

		if isTerminalActionStatus(status) {
			return status, nil, attempts, lastErr
		}

		if !qos.ShouldRetry(status) {
			return status, nil, attempts, lastErr
		}

		// Wait before the next attempt, honoring ctx cancellation.
		if waited := waitForBackoff(ctx, qos.Backoff(attempts-1)); waited != nil {
			return status, nil, attempts, waited
		}
	}
	return status, nil, attempts - 1, lastErr
}

// isTerminalActionStatus identifies statuses that should not be retried —
// success and intentional pauses (waiting/paused).
func isTerminalActionStatus(s core.ActionStatus) bool {
	switch s {
	case core.ActionSucceeded, core.ActionWaiting, core.ActionPaused:
		return true
	default:
		return false
	}
}

// waitForBackoff sleeps for the supplied duration unless ctx is cancelled
// first. Returns ctx.Err() on cancellation, nil when the wait completes.
// A non-positive duration short-circuits to nil.
func waitForBackoff(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// recordActionFailure surfaces the underlying error onto the process so
// callers can read it from p.Failure() once the process terminates.
func (p *AgentProcess) recordActionFailure(actionName string, err error) {
	if err != nil {
		p.setFailure(err)
		return
	}

	if p.Failure() == nil {
		p.setFailure(fmt.Errorf("action %q failed without an explicit error", actionName))
	}
}

// runWithPanicRecovery isolates the user's action from the runtime: panics
// are downgraded to ActionFailed and recorded as errors so the rest of the
// process can keep running (or fail gracefully).
func runWithPanicRecovery(ctx context.Context, action core.Action, pc *core.ProcessContext) (status core.ActionStatus) {
	defer func() {
		if r := recover(); r != nil {
			pc.RecordPanic(r)
			status = core.ActionFailed
		}
	}()
	return action.Execute(ctx, pc)
}

// buildProcessContext assembles a fresh ProcessContext for one tick. The
// fields all live on AgentProcess; we re-create the context every tick so
// per-action state (lastErr, etc.) doesn't leak.
func (p *AgentProcess) buildProcessContext() *core.ProcessContext {
	pc := &core.ProcessContext{
		Process:       p,
		Blackboard:    p.blackboard,
		Options:       p.options,
		OutputChannel: p.options.OutputChannel,
		Services:      p.platformServices(),
	}
	pc.SetPublishFunc(p.publishAny)

	if resolver := p.platformToolResolver(); resolver != nil {
		pc.SetResolveToolsFunc(resolveToolsFor(resolver))
	}
	return pc
}

func (p *AgentProcess) platformServices() *core.ServiceProvider {
	if p.platform == nil {
		return core.NewServiceProvider()
	}
	return p.platform.services
}

func (p *AgentProcess) platformToolResolver() core.ToolGroupResolver {
	if p.platform == nil || p.platform.services == nil {
		return nil
	}
	return p.platform.services.Tools
}

// resolveToolsFor builds the closure handed to ProcessContext for lazy
// tool resolution. Roles that don't resolve are skipped silently — the
// action decides whether the missing tools are fatal.
func resolveToolsFor(resolver core.ToolGroupResolver) core.ToolResolver {
	return func(ctx context.Context, roles []string) ([]core.AgentTool, error) {
		var collected []core.AgentTool

		for _, role := range roles {
			group, err := resolver.Resolve(ctx, core.ToolGroupRequirement{Role: role})
			if err != nil {
				return nil, fmt.Errorf("resolve tool group %q: %w", role, err)
			}
			if group == nil {
				continue
			}

			tools, err := group.Tools(ctx)
			if err != nil {
				return nil, fmt.Errorf("load tools for group %q: %w", role, err)
			}
			collected = append(collected, tools...)
		}
		return collected, nil
	}
}

