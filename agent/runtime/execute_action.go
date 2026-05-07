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
// The retry loop respects ActionQoS: ActionFailed retries up to MaxAttempts
// with back-off; ActionWaiting/Paused/Succeeded short-circuit immediately.
// The full retry loop (not each attempt) is wrapped by every registered
// [core.ActionInterceptor] — interceptors fire once per action, not per
// retry, matching embabel's AgentProcessCallback semantics.
func (p *AgentProcess) executeAction(ctx context.Context, action core.Action) (core.ActionStatus, *core.ReplanRequest) {
	meta := action.Metadata()
	startedAt := core.Now()

	p.publishEvent(event.ActionExecutionStartEvent{
		BaseEvent: p.baseEvent(),
		Action:    action,
		StartedAt: startedAt,
	})

	ctx, span := core.AgentTracer().Start(ctx, spanAction)
	span.SetAttributes(
		attribute.String(attrActionName, meta.Name),
		attribute.String(attrProcessID, p.id),
	)
	defer span.End()

	processContext := p.buildProcessContext(meta.ToolGroups, action)

	var (
		status   core.ActionStatus
		replan   *core.ReplanRequest
		attempts int
		lastErr  error
	)
	interceptors := collectActionInterceptors(p.combinedExtensions())
	status = runActionInterceptors(interceptors, ctx, p, action, func() core.ActionStatus {
		s, r, a, err := p.runWithRetry(ctx, action, processContext, meta.QoS)
		replan, attempts, lastErr = r, a, err
		return s
	})

	duration := time.Since(startedAt)

	p.state.recordInvocation(ActionInvocation{
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
		attribute.String(attrActionStatus, status.String()),
		attribute.Int(attrActionAttempts, attempts),
	)
	finishSpanWithError(span, lastErr)

	p.publishEvent(event.ActionExecutionResultEvent{
		BaseEvent: p.baseEvent(),
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
	processContext *core.ProcessContext,
	qos core.ActionQoS,
) (status core.ActionStatus, replan *core.ReplanRequest, attempts int, lastErr error) {
	op := func() error {
		attempts++
		processContext.ResetError()

		status = processContext.ExecuteSafely(ctx, action)
		lastErr = processContext.LastError()

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
		return actionFailureError(action.Metadata().Name)
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
		p.state.setFailure(err)
		return
	}

	if p.Failure() == nil {
		p.state.setFailure(actionFailureError(actionName))
	}
}

// buildProcessContext assembles a fresh ProcessContext for one tick. The
// fields all live on AgentProcess; we re-create the context every tick so
// per-action state (lastErr, etc.) doesn't leak. actionToolGroups is
// the currently-executing action's declared requirements; threading it
// in so [core.ProcessContext.ActionTools] can resolve them lazily.
//
// The action argument lets the runtime build a ResolveTools closure that
// runs every registered [core.ToolGroupResolver] in chain and decorates
// the produced tools with every registered [core.ToolDecorator] —
// without exposing those types to ProcessContext consumers.
func (p *AgentProcess) buildProcessContext(actionToolGroups []core.ToolGroupRequirement, action core.Action) *core.ProcessContext {
	config := core.ProcessContextConfig{
		Process:          p,
		Blackboard:       p.blackboard,
		Options:          p.options,
		OutputChannel:    p.options.OutputChannel,
		Services:         p.platformServices(),
		ActionToolGroups: actionToolGroups,
		Publish:           p.publishAny,
		ToolCallCancel:    p.signals.registerToolCallCancel,
		ResolveTools:      p.toolResolverFor(action),
	}
	return core.NewProcessContext(config)
}

func (p *AgentProcess) platformServices() *core.ServiceProvider {
	if p.platform == nil {
		return core.NewServiceProvider()
	}
	return p.platform.services
}

// platformExtensions exposes the platform-scoped extension list.
func (p *AgentProcess) platformExtensions() []core.Extension {
	if p.platform == nil {
		return nil
	}
	return p.platform.extensions.list
}

// processExtensions exposes the per-process extension list (from
// [core.ProcessOptions.Extensions]). Independent of platform extensions
// — combine via [combinedExtensions] / [combinedExtensionsResolverFirst]
// when an extension point needs both layers.
func (p *AgentProcess) processExtensions() []core.Extension {
	if p.options == nil {
		return nil
	}
	return p.options.Extensions
}

// combinedExtensions returns platform extensions followed by process
// extensions — the natural ordering for onion / wrap chains where
// platform sits outermost (registered earliest) and process sits
// innermost (registered last). Goal-approver dispatch and the
// last-wins singleton scans (rare for combined-scope) read this list.
func (p *AgentProcess) combinedExtensions() []core.Extension {
	platform := p.platformExtensions()
	process := p.processExtensions()
	if len(process) == 0 {
		return platform
	}
	out := make([]core.Extension, 0, len(platform)+len(process))
	out = append(out, platform...)
	out = append(out, process...)
	return out
}

// combinedExtensionsResolverFirst returns process extensions BEFORE
// platform extensions — the order used for first-hit resolvers so a
// process-scope override is consulted first.
func (p *AgentProcess) combinedExtensionsResolverFirst() []core.Extension {
	platform := p.platformExtensions()
	process := p.processExtensions()
	if len(process) == 0 {
		return platform
	}
	out := make([]core.Extension, 0, len(platform)+len(process))
	out = append(out, process...)
	out = append(out, platform...)
	return out
}

// toolResolverFor returns the ResolveTools closure used by ProcessContext.
// nil action is allowed (the resolver still works; ToolDecorators receive
// nil action — they should treat it as "outside an action body").
//
// Resolvers are walked process-first (so a process-scope override beats
// the platform default); decorators wrap platform-first then
// process-last (so a process-scope decorator is the outermost wrap and
// runs after platform decorators).
func (p *AgentProcess) toolResolverFor(action core.Action) core.ToolResolver {
	resolvers := collectToolGroupResolvers(p.combinedExtensionsResolverFirst())
	decorators := collectToolDecorators(p.combinedExtensions())
	if len(resolvers) == 0 {
		return nil
	}
	return func(ctx context.Context, roles []string) ([]core.AgentTool, error) {
		var collected []core.AgentTool

		for _, role := range roles {
			group, err := runToolGroupResolvers(resolvers, ctx, core.ToolGroupRequirement{Role: role})
			if err != nil {
				return nil, fmt.Errorf("resolve tools for role %q: %w", role, err)
			}
			if group == nil {
				continue
			}

			tools, err := group.Tools(ctx)
			if err != nil {
				return nil, fmt.Errorf("load tools for role %q: %w", role, err)
			}
			for _, tool := range tools {
				collected = append(collected, runToolDecorators(decorators, p, action, tool))
			}
		}
		return collected, nil
	}
}
