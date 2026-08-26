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
	OnModelResponse(ctx context.Context, invocation ModelInvocation, response *chat.Response)
	OnToolStarted(ctx context.Context, invocation ToolInvocation)
	OnToolSettled(ctx context.Context, invocation ToolInvocation, settlement ToolSettlement)
}

// ToolSettlement is the conclusive outcome visible at the Tool host boundary.
// Result is the exact value fed back to the model. InputRequired instead means
// the Tool paused durably before producing a result. Failure is reserved for a
// host/cancellation failure for which no ordinary ToolResult was produced.
type ToolSettlement struct {
	// Result is the exact ordinary Tool result fed back to the model.
	Result *chat.ToolResult
	// InputRequired reports that the Tool paused before producing Result.
	InputRequired bool
	// Failure describes a host or cancellation failure that produced no Result.
	Failure string
	// Unknown reports that the external Tool settlement could not be determined.
	Unknown bool
}

func (d *Dispatcher) observeModel(ctx context.Context, invocation ModelInvocation, response *chat.Response) {
	if d.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	d.observer.OnModelResponse(ctx, invocation, response.Clone())
}

func (d *Dispatcher) observeToolStarted(ctx context.Context, invocation ToolInvocation) {
	if d.observer == nil {
		return
	}
	defer func() { _ = recover() }()
	d.observer.OnToolStarted(ctx, invocation)
}

func (d *Dispatcher) observeToolSettled(ctx context.Context, invocation ToolInvocation, settlement ToolSettlement) {
	if d.observer == nil {
		return
	}
	if settlement.Result != nil {
		settlement.Result = new(*settlement.Result)
	}
	defer func() { _ = recover() }()
	d.observer.OnToolSettled(ctx, invocation, settlement)
}
