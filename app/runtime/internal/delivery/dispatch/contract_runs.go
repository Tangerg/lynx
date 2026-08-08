package dispatch

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerRuns(r *Registry) {
	// runs.start and runs.resume open a run. A same-key retry must land back on
	// THAT run — replaying the cached ack alone would give the client a runId with
	// no stream behind it (TRANSPORT §6.2).
	RunStreamCommand(r, MethodMeta{
		Name: "runs.start",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrSessionHasActiveRun.Error(),
			protocol.ErrUnsupportedMime.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.StartRun(ctx, in)
	}, runEventFramer)

	RunStreamCommand(r, MethodMeta{
		Name: "runs.resume",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrInterruptNotOpen.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.ResumeRun(ctx, in)
	}, runEventFramer)

	// runs.subscribe opens no run, so a retry is just another subscription.
	//
	// Its refusals are the vocabulary of addressing one live segment: the run is a
	// child, is waiting, has finished, has moved on, or the caller's replay cursor
	// cannot be served. Each is declared because each sends the client somewhere
	// different — rootRunId, interrupt.list, items.list, runs.get, or a cursorless
	// reattach — and one collapsed run_not_found would send it nowhere.
	Subscription(r, MethodMeta{
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
	}, func(d *Router, ctx context.Context, in protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return d.api.SubscribeRun(ctx, in)
	}, runEventFramer)

	Command(r, MethodMeta{
		Name: "runs.cancel",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunFinished.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
		return d.api.CancelRun(ctx, in)
	})

	// A steer addresses the same thing a subscribe does — one live segment — so it
	// refuses with the same vocabulary. There is no best-effort injection: a run
	// that has parked, finished or moved to another segment says so, and the client
	// asks the user again rather than delivering an instruction to work they never
	// saw.
	CommandAck(r, MethodMeta{
		Name: "runs.steer",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunNotRoot.Error(),
			protocol.ErrRunWaiting.Error(),
			protocol.ErrRunFinished.Error(),
			protocol.ErrStaleSegment.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.SteerRunRequest) error {
		return d.api.SteerRun(ctx, in)
	})

	// runs.get answers "what is this run" for a runId a client already holds — from
	// an event, a page, or a link — without it having to know the session first.
	Query(r, MethodMeta{
		Name: "runs.get",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.GetRunRequest) (*protocol.RunRef, error) {
		return d.api.GetRun(ctx, in)
	})

	// Only a request that asks for descendants needs features.subagents; the
	// default page of root runs is always available. The condition treats
	// `includeDescendants: false` as "not asking", so an explicit false and an
	// absent field behave alike — while an explicit true is refused rather than
	// read as false, which would hand back a page that looks complete and is not.
	Query(r, MethodMeta{
		Name: "runs.list",
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
		Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
		return d.api.ListRuns(ctx, in)
	})
}
