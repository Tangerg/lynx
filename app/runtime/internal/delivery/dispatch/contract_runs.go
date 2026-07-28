package dispatch

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerLifecycle(r *Registry) {
	// runtime.discover takes no params; struct{} makes an unexpected field a
	// decode failure rather than something silently ignored.
	Unary(r, MethodMeta{Name: "runtime.discover", Stability: stable},
		func(d *Dispatcher, ctx context.Context, _ struct{}) (*protocol.DiscoverResponse, error) {
			return d.api.Discover(ctx)
		})
}

func registerRuns(r *Registry) {
	// runs.start and runs.resume open a run. A same-key retry must land back on
	// THAT run — replaying the cached ack alone would give the client a runId with
	// no stream behind it (TRANSPORT §6.2).
	Stream(r, MethodMeta{
		Name:        "runs.start",
		Idempotency: IdempotencyReplayRunStream,
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrUnsupportedMime.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.StartRun(ctx, in)
	}, runEventFramer)

	Stream(r, MethodMeta{
		Name:        "runs.resume",
		Idempotency: IdempotencyReplayRunStream,
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrInterruptNotOpen.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ResumeRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.ResumeRun(ctx, in)
	}, runEventFramer)

	// runs.subscribe opens no run, so a retry is just another subscription.
	Stream(r, MethodMeta{
		Name:      "runs.subscribe",
		Errors:    []string{protocol.ErrRunNotFound.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SubscribeRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.SubscribeRun(ctx, in.RunID)
	}, runEventFramer)

	UnaryAck(r, MethodMeta{
		Name:        "runs.cancel",
		Idempotency: IdempotencyReplayResponse,
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunAlreadyDone.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CancelRunRequest) error {
		return d.api.CancelRun(ctx, in)
	})

	UnaryAck(r, MethodMeta{
		Name:        "runs.steer",
		Idempotency: IdempotencyReplayResponse,
		Errors:      []string{protocol.ErrRunNotFound.Error()},
		Stability:   stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SteerRunRequest) error {
		return d.api.SteerRun(ctx, in)
	})

	Unary(r, MethodMeta{Name: "runs.list", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
			return d.api.ListRuns(ctx, in)
		})

	Unary(r, MethodMeta{Name: "runs.listOpenInterrupts", Stability: stable},
		func(d *Dispatcher, ctx context.Context, in protocol.ListOpenInterruptsRequest) (*protocol.Page[protocol.OpenInterrupt], error) {
			return d.api.ListOpenInterrupts(ctx, in)
		})
}

func registerItems(r *Registry) {
	Unary(r, MethodMeta{
		Name:      "items.list",
		Errors:    []string{protocol.ErrSessionNotFound.Error()},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
		return d.api.ListItems(ctx, in)
	})
}
