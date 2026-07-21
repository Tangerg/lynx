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
)

// Tracing attribute / span keys local to action execution.
const (
	spanAction       = "agent.action"
	attrAction       = "agent.action.name"
	attrActionStatus = "agent.action.status"
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
		ActionTools:   p.toolResolverFor(action),
		RunInteraction: func(ctx context.Context, input core.Interaction) (interaction.Result, error) {
			return p.runInteraction(ctx, action.Metadata().Name, input)
		},
		ToolCallCancel:   p.signals.registerToolCallCancel,
		ActionToolGroups: actionToolGroups,
	}
	return core.NewProcessContext(config)
}

// executeAction invokes one Action with panic recovery and post-action
// bookkeeping (history record, action-run condition, events). It returns the
// ActionStatus plus an optional ReplanRequest raised by the action. Retry
// policy belongs to the operation implementation that understands its own
// side effects; the framework never replays an Action.
func (p *Process) executeAction(ctx context.Context, action core.Action) (core.ActionStatus, *core.ReplanRequest, error) {
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

	status, lastErr := p.invokeActionChain(ctx, action, func() (core.ActionStatus, error) {
		return p.invokeAction(ctx, action, processContext)
	})
	status, lastErr = validateActionResult(metadata.Name, status, lastErr)
	if status == core.ActionFailed && lastErr == nil {
		lastErr = p.actionFailure(metadata.Name)
	}
	var replan *core.ReplanRequest
	if _, invalid := errors.AsType[invalidActionResultError](lastErr); !invalid {
		if request, ok := errors.AsType[*core.ReplanRequest](lastErr); ok {
			replan = request
		}
	}
	if aborted, cleanupErr := p.abortStagedNestedChildren(ctx); aborted > 0 {
		if status != core.ActionFailed {
			status = core.ActionFailed
			lastErr = errors.New("runtime: action returned without committing its staged nested child suspensions")
			replan = nil
		}
		lastErr = errors.Join(lastErr, cleanupErr)
	}

	duration := time.Since(startedAt)
	p.recordActionMetric(ctx, status, duration)

	p.state.recordActionRun(ActionRun{
		ActionName: metadata.Name,
		StartedAt:  startedAt,
		Duration:   duration,
		Status:     status,
	})

	if status == core.ActionSucceeded {
		// The action-run condition gates non-repeatable actions. Set it only on
		// success so a future plan may still select an unsuccessful action.
		p.blackboard.StoreCondition(metadata.RunCondition(), true)
	}

	span.SetAttributes(attribute.String(attrActionStatus, status.String()))
	finishSpanWithError(span, lastErr)

	p.publishEvent(ctx, event.ActionFinished{
		Header:   p.eventHeader(),
		Action:   action,
		Status:   status,
		Duration: duration,
		Err:      lastErr,
	})

	return status, replan, lastErr
}

type invalidActionResultError struct {
	action string
	status core.ActionStatus
	reason string
}

func (e invalidActionResultError) Error() string {
	if e.reason != "" {
		return fmt.Sprintf("runtime: action %q returned invalid result: status %s %s", e.action, e.status, e.reason)
	}
	return fmt.Sprintf("runtime: action %q returned invalid status %d", e.action, e.status)
}

func validateActionResult(action string, status core.ActionStatus, err error) (core.ActionStatus, error) {
	if !status.Valid() {
		return core.ActionFailed, errors.Join(err, invalidActionResultError{action: action, status: status})
	}
	if err != nil && status != core.ActionFailed {
		return core.ActionFailed, errors.Join(err, invalidActionResultError{
			action: action,
			status: status,
			reason: "must not carry an error",
		})
	}
	return status, err
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

func (p *Process) invokeActionChain(
	ctx context.Context,
	action core.Action,
	base func() (core.ActionStatus, error),
) (status core.ActionStatus, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			status = core.ActionFailed
			err = panicerr.New("runtime.Process.executeAction: action middleware panicked", recovered)
		}
	}()
	return p.runActionChain(ctx, action, base)
}

// actionFailure produces a default failure error when an action returned
// ActionFailed without recording an explicit error on the ProcessContext.
func (p *Process) actionFailure(name string) error {
	return fmt.Errorf("runtime.Process %q: action %q failed without recording an error", p.ID(), name)
}
