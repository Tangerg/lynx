package embedded

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// StartRun opens a root Run and returns its live event stream.
func (r *Runtime) StartRun(ctx context.Context, request protocol.StartRunRequest, options RunCommandOptions) (*protocol.StartRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.invokeStream[protocol.StartRunRequest, *protocol.StartRunResponse, protocol.RunEvent](ctx, operation.RunsStart, request, runCommandOptions(options))
}

// ResumeRun answers a waiting Run tree and returns its next Segment stream.
func (r *Runtime) ResumeRun(ctx context.Context, request protocol.ResumeRunRequest, options RunCommandOptions) (*protocol.ResumeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.invokeStream[protocol.ResumeRunRequest, *protocol.ResumeRunResponse, protocol.RunEvent](ctx, operation.RunsResume, request, runCommandOptions(options))
}

// SubscribeRun attaches to one live root Segment, optionally after a replay cursor.
func (r *Runtime) SubscribeRun(ctx context.Context, request protocol.SubscribeRunRequest, options RunSubscriptionOptions) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	return r.invokeStream[protocol.SubscribeRunRequest, *protocol.SubscribeRunResponse, protocol.RunEvent](ctx, operation.RunsSubscribe, request, runSubscriptionOptions(options))
}

// CancelRun requests cancellation of a Run or Run subtree.
func (r *Runtime) CancelRun(ctx context.Context, request protocol.CancelRunRequest, options CommandOptions) (*protocol.CancelRunResponse, error) {
	return r.invoke[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, operation.RunsCancel, request, commandOptions(options))
}

// SteerRun queues an instruction at the addressed Segment's next safe boundary.
func (r *Runtime) SteerRun(ctx context.Context, request protocol.SteerRunRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.RunsSteer, request, commandOptions(options))
}

// GetRun returns one Run projection by identity.
func (r *Runtime) GetRun(ctx context.Context, request protocol.GetRunRequest, options CallOptions) (*protocol.RunRef, error) {
	return r.invoke[protocol.GetRunRequest, *protocol.RunRef](ctx, operation.RunsGet, request, callOptions(options))
}

// ListRuns returns one cursor page of Runs.
func (r *Runtime) ListRuns(ctx context.Context, request protocol.ListRunsRequest, options CallOptions) (*protocol.Page[protocol.RunRef], error) {
	return r.invoke[protocol.ListRunsRequest, *protocol.Page[protocol.RunRef]](ctx, operation.RunsList, request, callOptions(options))
}
