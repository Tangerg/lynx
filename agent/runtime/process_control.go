package runtime

import (
	"context"
	"errors"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
)

// TerminateAgent queues a "stop the whole process" signal. Tick consumes
// the channel at the next boundary.
func (p *AgentProcess) TerminateAgent(reason string) {
	p.signals.queueTermination(core.TerminationScopeAgent, reason)
}

// TerminateAction queues a "skip the current action and re-plan" signal.
func (p *AgentProcess) TerminateAction(reason string) {
	p.signals.queueTermination(core.TerminationScopeAction, reason)
}

// TerminateToolCall fires the cancel func of the most recently derived
// tool-call context (if any). Action bodies that derive their tool
// invocation contexts via [core.ProcessContext.ToolCallContext] observe
// ctx.Done() at this point and abort. No-op when no tool-call context
// is currently registered.
func (p *AgentProcess) TerminateToolCall() {
	p.signals.fireToolCallCancel()
}

// PendingAwaitable returns the awaitable currently parked by an
// in-flight [AgentProcess.AwaitInput], or nil when nothing is parked.
// Used by supervisor patterns that want to surface a suspended
// child's pending request back to the parent LLM as tool-result text
// rather than failing the parent.
func (p *AgentProcess) PendingAwaitable() core.Awaitable {
	return p.signals.peekAwaitable()
}

// AwaitInput parks the supplied awaitable on the process and returns
// [core.ActionWaiting] so the calling action's typed-action wrapper
// transitions the process to [core.StatusWaiting]. The action's tick
// loop exits at that boundary; the user resumes the process by calling
// [Platform.ResumeProcess], which routes the response through
// awaitable.OnResponseAny — typically mutating the blackboard so the
// next planning tick sees fresh state.
func (p *AgentProcess) AwaitInput(req core.Awaitable) core.ActionStatus {
	return p.AwaitInputContext(context.Background(), req)
}

// AwaitInputContext is the context-aware companion to [AgentProcess.AwaitInput].
// It preserves the caller's trace when publishing the waiting event.
func (p *AgentProcess) AwaitInputContext(ctx context.Context, req core.Awaitable) core.ActionStatus {
	if ctx == nil {
		ctx = context.Background()
	}
	status := p.signals.parkAwaitable(req)
	if status == core.ActionWaiting {
		p.publishEvent(ctx, event.ProcessWaiting{BaseEvent: p.baseEvent(), Awaitable: req})
		return status
	}
	if req != nil {
		// parkAwaitable refused a non-nil request: an awaitable is already
		// pending (two AwaitInput in one tick — e.g. ProcessConcurrent with two
		// interrupting actions). Record the reason so the run loop surfaces it
		// rather than the generic "action failed without an error".
		p.state.setFailure(errors.New("runtime: an awaitable is already pending; one HITL interrupt per process (concurrent interrupts unsupported)"))
	}
	return status
}
