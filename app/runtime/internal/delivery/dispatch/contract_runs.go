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
			protocol.ErrSessionHasActiveRun.Error(),
			protocol.ErrUnsupportedMime.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
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
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.ResumeRun(ctx, in)
	}, runEventFramer)

	// runs.subscribe opens no run, so a retry is just another subscription.
	//
	// Its refusals are the vocabulary of addressing one live segment: the run is a
	// child, is waiting, has finished, has moved on, or the caller's replay cursor
	// cannot be served. Each is declared because each sends the client somewhere
	// different — rootRunId, interrupts.list, items.list, runs.get, or a cursorless
	// reattach — and one collapsed run_not_found would send it nowhere.
	Stream(r, MethodMeta{
		Name: "runs.subscribe",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunNotRoot.Error(),
			protocol.ErrRunWaiting.Error(),
			protocol.ErrRunFinished.Error(),
			protocol.ErrStaleSegment.Error(),
			protocol.ErrReplayCursorInvalid.Error(),
			protocol.ErrReplayUnavailable.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.SubscribeRun(ctx, in)
	}, runEventFramer)

	Unary(r, MethodMeta{
		Name:        "runs.cancel",
		Idempotency: IdempotencyReplayResponse,
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunFinished.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
		return d.api.CancelRun(ctx, in)
	})

	// A steer addresses the same thing a subscribe does — one live segment — so it
	// refuses with the same vocabulary. There is no best-effort injection: a run
	// that has parked, finished or moved to another segment says so, and the client
	// asks the user again rather than delivering an instruction to work they never
	// saw.
	UnaryAck(r, MethodMeta{
		Name:        "runs.steer",
		Idempotency: IdempotencyReplayResponse,
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunNotRoot.Error(),
			protocol.ErrRunWaiting.Error(),
			protocol.ErrRunFinished.Error(),
			protocol.ErrStaleSegment.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.SteerRunRequest) error {
		return d.api.SteerRun(ctx, in)
	})

	// runs.get answers "what is this run" for a runId a client already holds — from
	// an event, a page, or a link — without it having to know the session first.
	Unary(r, MethodMeta{
		Name: "runs.get",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GetRunRequest) (*protocol.RunRef, error) {
		return d.api.GetRun(ctx, in)
	})

	// Only a request that asks for descendants needs features.subagents; the
	// default page of root runs is always available. The condition treats
	// `includeDescendants: false` as "not asking", so an explicit false and an
	// absent field behave alike — while an explicit true is refused rather than
	// read as false, which would hand back a page that looks complete and is not.
	Unary(r, MethodMeta{
		Name: "runs.list",
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
		return d.api.ListRuns(ctx, in)
	})
}

func registerInterrupts(r *Registry) {
	// interrupts.list is its own root because a waiting set belongs to a run TREE,
	// not to one run: it was runs.listOpenInterrupts, which read as "one run's
	// interrupts" and made the aggregate look like a per-run detail.
	//
	// run_not_root is declared because the filter can name a child, and that is a
	// different answer from "nothing is waiting".
	Unary(r, MethodMeta{
		Name: "interrupts.list",
		Errors: []string{
			protocol.ErrRunNotRoot.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
		return d.api.ListInterrupts(ctx, in)
	})
}

func registerTodos(r *Registry) {
	// The recovery source the todos state key declares. A session with no list yet
	// answers with the empty state at revision 0 — "nothing written" is a fact, and
	// only a session that does not exist is an error.
	Unary(r, MethodMeta{
		Name:            "todos.get",
		Errors:          []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: requires(protocol.FeatureTodos),
		Stability:       stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.GetTodosRequest) (*protocol.StateSnapshot, error) {
		return d.api.GetTodos(ctx, in)
	})
}

func registerItems(r *Registry) {
	// Either scope can name something that does not exist, and the client's next move
	// differs — find the session, or find the run — so both refusals are declared.
	// Asking a run scope for its subtree needs features.subagents; the scope itself
	// does not, since a root run is a run.
	Unary(r, MethodMeta{
		Name: "items.list",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrRunNotFound.Error(),
		},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "scope.includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
		Stability: stable,
	}, func(d *Dispatcher, ctx context.Context, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
		return d.api.ListItems(ctx, in)
	})
}
