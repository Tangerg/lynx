package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/internal/panicerr"
	"github.com/Tangerg/lynx/pkg/retry"
)

// Tracing attribute / span keys local to action execution.
const (
	spanAction         = "agent.action"
	attrAction         = "agent.action.name"
	attrActionStatus   = "agent.action.status"
	attrActionAttempts = "agent.action.attempts"
)

// buildProcessContext assembles the action-scoped capabilities exposed to one
// execution. A fresh value keeps tool requirements and interaction state from
// leaking between actions.
func (p *Process) buildProcessContext(actionToolGroups []core.ToolGroupRequirement, action core.Action) *core.ProcessContext {
	maxToolRounds := 0
	if guardrails := p.effectiveGuardrails(); guardrails != nil {
		maxToolRounds = guardrails.MaxToolRounds
	}
	config := core.ProcessContextConfig{
		Process:       p,
		Control:       processControl{process: p},
		Usage:         processUsage{process: p},
		Blackboard:    p.blackboard,
		Session:       p.options.session,
		Dependencies:  p.dependencies.Child(),
		Chat:          p.effectiveChat,
		MaxToolRounds: maxToolRounds,
		Emit:          p.publishAny,
		ResolveTools:  p.toolResolverFor(action),
		RunInteraction: func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return p.runInteraction(ctx, action.Metadata().Name, input)
		},
		ToolCallCancel:   p.signals.registerToolCallCancel,
		ActionToolGroups: actionToolGroups,
	}
	return core.NewProcessContext(config)
}

// executeAction runs a single Action with retry, panic recovery, and
// post-action bookkeeping (history record, action-run condition, events). It
// returns the final ActionStatus plus an optional ReplanRequest the action
// raised.
//
// The retry loop respects RetryPolicy: an explicitly replay-safe ActionFailed
// retries up to MaxAttempts with back-off; ActionWaiting/Paused/Succeeded,
// cancellation, replan, and committed interaction errors short-circuit.
// The full retry loop (not each attempt) is wrapped by every registered
// [core.ActionMiddleware] — actionMiddleware fire once per action, not per
// retry, matching standard agent-process callback semantics.
func (p *Process) executeAction(ctx context.Context, action core.Action) (core.ActionStatus, *core.ReplanRequest) {
	metadata := action.Metadata()
	startedAt := time.Now()

	p.publishEvent(ctx, event.ActionStarted{
		Header:    p.eventHeader(),
		Action:    action,
		StartedAt: startedAt,
	})

	ctx, span := agentTracer.Start(ctx, spanAction)
	span.SetAttributes(
		attribute.String(attrAction, metadata.Name),
		attribute.String(attrProcessID, p.id),
	)
	defer span.End()

	processContext := p.buildProcessContext(metadata.ToolGroups, action)

	var (
		status   core.ActionStatus
		replan   *core.ReplanRequest
		attempts int
		lastErr  error
	)
	status, lastErr = p.runActionChain(ctx, action, func() (core.ActionStatus, error) {
		finalStatus, replanRequest, attemptCount, err := p.runWithRetry(ctx, action, processContext, metadata.Retry)
		replan, attempts = replanRequest, attemptCount
		return finalStatus, err
	})
	status, lastErr = validateActionResult(metadata.Name, status, lastErr)
	if _, invalid := errors.AsType[invalidActionStatusError](lastErr); !invalid {
		if request, ok := errors.AsType[*core.ReplanRequest](lastErr); ok {
			replan = request
		}
	} else {
		replan = nil
	}
	if p.abortStagedNestedChildren(ctx) > 0 && status != core.ActionFailed {
		status = core.ActionFailed
		lastErr = errors.New("runtime: action returned without committing its staged nested child suspensions")
		replan = nil
	}

	duration := time.Since(startedAt)
	p.recordActionMetric(ctx, status, duration)

	p.state.recordActionRun(ActionRun{
		ActionName: metadata.Name,
		StartedAt:  startedAt,
		Duration:   duration,
		Status:     status,
		Attempts:   attempts,
	})

	if status == core.ActionSucceeded {
		// The action-run condition gates non-repeatable actions; set it only on success so
		// retrying after a future re-plan remains possible.
		p.blackboard.StoreCondition(metadata.RunCondition(), true)
	}

	span.SetAttributes(
		attribute.String(attrActionStatus, status.String()),
		attribute.Int(attrActionAttempts, attempts),
	)
	finishSpanWithError(span, lastErr)

	p.publishEvent(ctx, event.ActionFinished{
		Header:   p.eventHeader(),
		Action:   action,
		Status:   status,
		Duration: duration,
		Err:      lastErr,
	})

	if status == core.ActionFailed && replan == nil {
		p.recordActionFailure(metadata.Name, lastErr)
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

type invalidActionStatusError struct {
	action string
	status core.ActionStatus
}

func (e invalidActionStatusError) Error() string {
	return fmt.Sprintf("runtime: action %q returned invalid status %d", e.action, e.status)
}

func validateActionResult(action string, status core.ActionStatus, err error) (core.ActionStatus, error) {
	if status.Valid() {
		return status, err
	}
	return core.ActionFailed, errors.Join(err, invalidActionStatusError{action: action, status: status})
}

// runWithRetry runs action up to policy.MaxAttempts times, delegating the
// retry orchestration (timing, jitter, ctx-cancellation) to
// [github.com/Tangerg/lynx/pkg/retry]. The Operation closure captures
// per-attempt outcomes so the caller can inspect the final state without
// re-parsing the wrapped retry error.
func (p *Process) runWithRetry(
	ctx context.Context,
	action core.Action,
	processContext *core.ProcessContext,
	policy core.RetryPolicy,
) (status core.ActionStatus, replan *core.ReplanRequest, attempts int, lastErr error) {
	effects := action.Metadata().Effects
	attempt := func() error {
		attempts++

		// On a retry (any attempt after the first), clear this action's
		// declared effect conditions so a half-applied effect from the failed
		// attempt doesn't carry into the next one.
		if attempts > 1 {
			for key := range effects {
				p.blackboard.StoreCondition(key, false)
			}
		}

		status, lastErr = p.invokeAction(ctx, action, processContext)

		if request, ok := errors.AsType[*core.ReplanRequest](lastErr); ok {
			replan = request
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
		return p.actionFailure(action.Metadata().Name)
	}

	maxAttempts := policy.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = core.DefaultRetryPolicy().MaxAttempts
	}

	// Discard the retry error — status, replan, attempts, and lastErr
	// are captured by the attempt closure above. retry.Do's error only signals
	// that retries were exhausted, which the captured state already reflects.
	_ = retry.Do(attempt,
		retry.WithContext(ctx),
		retry.WithMaxAttempts(maxAttempts),
		retry.WithBaseDelay(policy.BaseDelay),
		retry.WithMaxDelay(policy.MaxDelay),
		retry.WithExponentialBackoff(),
		retry.WithRetryCondition(shouldRetryAction),
	)
	return status, replan, attempts, lastErr
}

func (p *Process) invokeAction(ctx context.Context, action core.Action, processContext *core.ProcessContext) (status core.ActionStatus, err error) {
	if action == nil {
		return core.ActionFailed, errors.New("runtime.Process.invokeAction: action is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			status = core.ActionFailed
			err = panicerr.New("runtime.Process.invokeAction: action panicked", recovered)
		}
	}()
	status, err = action.Execute(ctx, processContext)
	return validateActionResult(action.Metadata().Name, status, err)
}

// actionFailure produces a default failure error when an action returned
// ActionFailed without recording an explicit error on the ProcessContext.
func (p *Process) actionFailure(name string) error {
	return fmt.Errorf("runtime.Process %q: action %q failed without recording an error", p.ID(), name)
}

// shouldRetryAction stops the retry loop on signals that mean "don't try
// again": replan requests (the planner needs to be re-consulted) and
// halt sentinels (the action paused or is awaiting input). Anything else
// — including a plain failure — is retryable.
func shouldRetryAction(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if _, ok := errors.AsType[*core.ReplanRequest](err); ok {
		return false
	}
	if _, ok := errors.AsType[haltSignal](err); ok {
		return false
	}
	if _, ok := errors.AsType[invalidActionStatusError](err); ok {
		return false
	}
	if errors.Is(err, interaction.ErrCommitted) {
		return false
	}
	return true
}

// recordActionFailure surfaces the underlying error onto the process so
// callers can read it from p.Failure() once the process terminates.
func (p *Process) recordActionFailure(actionName string, err error) {
	if err != nil {
		p.state.recordFailure(err)
		return
	}

	if p.Failure() == nil {
		p.state.recordFailure(p.actionFailure(actionName))
	}
}
