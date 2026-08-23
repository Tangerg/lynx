package interaction

import (
	"context"

	"github.com/Tangerg/lynx/core/chat"
)

// ExecutionObserver receives exact, provider-neutral Interaction facts after
// model and Tool boundaries settle. It is observational: callbacks cannot
// alter execution, panics are isolated, and implementations must return in
// bounded time. A Dispatcher may invoke it concurrently for explicitly
// concurrent Tool calls.
type ExecutionObserver interface {
	OnModelResponse(context.Context, ModelInvocation, *chat.Response)
	OnToolStarted(context.Context, ToolInvocation)
	OnToolSettled(context.Context, ToolInvocation, ToolSettlement)
}

// ToolSettlement is the conclusive outcome visible at the Tool host boundary.
// Result is the exact value fed back to the model. InputRequired instead means
// the Tool paused durably before producing a result. Failure is reserved for a
// host/cancellation failure for which no ordinary ToolResult was produced.
type ToolSettlement struct {
	Result        *chat.ToolResult
	InputRequired bool
	Failure       string
	Unknown       bool
}

func (dispatcher *Dispatcher) observeModel(ctx context.Context, invocation ModelInvocation, response *chat.Response) {
	if dispatcher.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	dispatcher.observer.OnModelResponse(ctx, invocation, response.Clone())
}

func (dispatcher *Dispatcher) observeToolStarted(ctx context.Context, invocation ToolInvocation) {
	if dispatcher.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	dispatcher.observer.OnToolStarted(ctx, invocation)
}

func (dispatcher *Dispatcher) observeToolSettled(ctx context.Context, invocation ToolInvocation, settlement ToolSettlement) {
	if dispatcher.observer == nil {
		return
	}
	if settlement.Result != nil {
		settlement.Result = new(*settlement.Result)
	}
	defer func() { _ = recover() }()
	dispatcher.observer.OnToolSettled(ctx, invocation, settlement)
}
