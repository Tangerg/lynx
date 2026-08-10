package embedded

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// StartRun opens a root Run and returns its live event stream.
func (r *Runtime) StartRun(ctx context.Context, request protocol.StartRunRequest, options RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return invokeStream[protocol.StartRunRequest, *protocol.StartRunResponse, protocol.RunEvent](ctx, r, "runs.start", request, runCommandOptions(options))
}

// ResumeRun answers a waiting Run tree and returns its next Segment stream.
func (r *Runtime) ResumeRun(ctx context.Context, request protocol.ResumeRunRequest, options RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return invokeStream[protocol.ResumeRunRequest, *protocol.ResumeRunResponse, protocol.RunEvent](ctx, r, "runs.resume", request, runCommandOptions(options))
}

// SubscribeRun attaches to one live root Segment, optionally after a replay cursor.
func (r *Runtime) SubscribeRun(ctx context.Context, request protocol.SubscribeRunRequest, options RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return invokeStream[protocol.SubscribeRunRequest, *protocol.SubscribeRunResponse, protocol.RunEvent](ctx, r, "runs.subscribe", request, runSubscriptionOptions(options))
}

// CancelRun requests cancellation of a Run or Run subtree.
func (r *Runtime) CancelRun(ctx context.Context, request protocol.CancelRunRequest, options CommandOptions) (*protocol.CancelRunResponse, error) {
	return invoke[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, r, "runs.cancel", request, commandOptions(options))
}

// SteerRun queues an instruction at the addressed Segment's next safe boundary.
func (r *Runtime) SteerRun(ctx context.Context, request protocol.SteerRunRequest, options CommandOptions) error {
	return invokeAck(ctx, r, "runs.steer", request, commandOptions(options))
}

// GetRun returns one Run projection by identity.
func (r *Runtime) GetRun(ctx context.Context, request protocol.GetRunRequest, options CallOptions) (*protocol.RunRef, error) {
	return invoke[protocol.GetRunRequest, *protocol.RunRef](ctx, r, "runs.get", request, callOptions(options))
}

// ListRuns returns one cursor page of Runs.
func (r *Runtime) ListRuns(ctx context.Context, request protocol.ListRunsRequest, options CallOptions) (*protocol.Page[protocol.RunRef], error) {
	return invoke[protocol.ListRunsRequest, *protocol.Page[protocol.RunRef]](ctx, r, "runs.list", request, callOptions(options))
}

// ListInterrupts returns waiting interrupt sets for Run trees.
func (r *Runtime) ListInterrupts(ctx context.Context, request protocol.ListInterruptsRequest, options CallOptions) (*protocol.Page[protocol.PendingInterruptSet], error) {
	return invoke[protocol.ListInterruptsRequest, *protocol.Page[protocol.PendingInterruptSet]](ctx, r, "interrupts.list", request, callOptions(options))
}

// ListItems returns the authoritative transcript Items for a Session or Run scope.
func (r *Runtime) ListItems(ctx context.Context, request protocol.ListItemsRequest, options CallOptions) (*protocol.ListItemsResponse, error) {
	return invoke[protocol.ListItemsRequest, *protocol.ListItemsResponse](ctx, r, "items.list", request, callOptions(options))
}

// GetPlan returns the current Plan state snapshot for a Session.
func (r *Runtime) GetPlan(ctx context.Context, request protocol.GetPlanRequest, options CallOptions) (*protocol.StateSnapshot, error) {
	return invoke[protocol.GetPlanRequest, *protocol.StateSnapshot](ctx, r, "plan.get", request, callOptions(options))
}
