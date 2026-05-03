package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/pkg/retry"
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
		p.blackboard.SetCondition(meta.HasRunKey(), true)
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

// haltSignal is the sentinel error sent to [pkg/retry] when an action
// returns a non-failure non-success status (Waiting / Paused). It tells
// the retry loop to stop without treating the situation as a retryable
// failure.
type haltSignal struct{ status core.ActionStatus }

func (h haltSignal) Error() string {
	return "action halted with status " + h.status.String()
}

// runWithRetry runs action up to qos.MaxAttempts times, delegating the
// retry orchestration (timing, jitter, ctx-cancellation) to
// [github.com/Tangerg/lynx/pkg/retry]. The Operation closure captures
// per-attempt outcomes so the caller can inspect the final state without
// re-parsing the wrapped retry error.
func (p *AgentProcess) runWithRetry(
	ctx context.Context,
	action core.Action,
	pc *core.ProcessContext,
	qos core.ActionQos,
) (status core.ActionStatus, replan *core.ReplanRequest, attempts int, lastErr error) {
	op := func() error {
		attempts++
		pc.ResetError()

		status = runWithPanicRecovery(ctx, action, pc)
		lastErr = pc.LastError()

		if rr := core.AsReplanRequest(lastErr); rr != nil {
			replan = rr
			return lastErr
		}

		switch status {
		case core.ActionSucceeded:
			return nil
		case core.ActionWaiting, core.ActionPaused:
			return haltSignal{status: status}
		}

		// ActionFailed or any other non-terminal status — produce an
		// error so [pkg/retry] knows this attempt didn't succeed.
		if lastErr != nil {
			return lastErr
		}
		return fmt.Errorf("action %q failed without an explicit error", action.Metadata().Name)
	}

	maxAttempts := qos.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	_ = retry.Do(op,
		retry.WithContext(ctx),
		retry.WithMaxAttempts(maxAttempts),
		retry.WithBaseDelay(qos.BaseDelay),
		retry.WithMaxDelay(qos.MaxDelay),
		retry.WithExponentialBackoff(),
		retry.WithRetryCondition(shouldRetryAction),
	)
	return status, replan, attempts, lastErr
}

// shouldRetryAction stops the retry loop on signals that mean "don't try
// again": replan requests (the planner needs to be re-consulted) and
// halt sentinels (the action paused or is awaiting input). Anything else
// — including a plain failure — is retryable.
func shouldRetryAction(err error) bool {
	if core.AsReplanRequest(err) != nil {
		return false
	}
	var halt haltSignal
	if errors.As(err, &halt) {
		return false
	}
	return true
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
	if p.platform == nil {
		return nil
	}
	return p.platform.tools
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

