package operation

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerRuns(registry *Registry) {
	// runs.start and runs.resume open a run. A same-key retry must land back on
	// THAT run — replaying the cached ack alone would give the client a runId with
	// no stream behind it (TRANSPORT §6.2).
	RunStreamCommand(registry, MethodMeta{
		Name: "runs.start",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrSessionHasActiveRun.Error(),
			protocol.ErrUnsupportedMime.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(service interface {
		StartRun(context.Context, protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error)
	}, ctx context.Context, request protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
		return service.StartRun(ctx, request)
	})

	RunStreamCommand(registry, MethodMeta{
		Name: "runs.resume",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrInterruptNotOpen.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(service interface {
		ResumeRun(context.Context, protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error)
	}, ctx context.Context, request protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return service.ResumeRun(ctx, request)
	})

	// runs.subscribe opens no run, so a retry is just another subscription.
	//
	// Its refusals are the vocabulary of addressing one live segment: the run is a
	// child, is waiting, has finished, has moved on, or the caller's replay cursor
	// cannot be served. Each is declared because each sends the client somewhere
	// different — rootRunId, interrupt.list, items.list, runs.get, or a cursorless
	// reattach — and one collapsed run_not_found would send it nowhere.
	Subscription(registry, MethodMeta{
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
	}, func(service interface {
		SubscribeRun(context.Context, protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error)
	}, ctx context.Context, request protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return service.SubscribeRun(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "runs.cancel",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunFinished.Error(),
			protocol.ErrSessionBusy.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(service interface {
		CancelRun(context.Context, protocol.CancelRunRequest) (*protocol.CancelRunResponse, error)
	}, ctx context.Context, request protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
		return service.CancelRun(ctx, request)
	})

	// A steer addresses the same thing a subscribe does — one live segment — so it
	// refuses with the same vocabulary. There is no best-effort injection: a run
	// that has parked, finished or moved to another segment says so, and the client
	// asks the user again rather than delivering an instruction to work they never
	// saw.
	CommandAck(registry, MethodMeta{
		Name: "runs.steer",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrRunNotRoot.Error(),
			protocol.ErrRunWaiting.Error(),
			protocol.ErrRunFinished.Error(),
			protocol.ErrStaleSegment.Error(),
		},
		Stability: stable,
	}, func(service interface {
		SteerRun(context.Context, protocol.SteerRunRequest) error
	}, ctx context.Context, request protocol.SteerRunRequest) error {
		return service.SteerRun(ctx, request)
	})

	// runs.get answers "what is this run" for a runId a client already holds — from
	// an event, a page, or a link — without it having to know the session first.
	Query(registry, MethodMeta{
		Name: "runs.get",
		Errors: []string{
			protocol.ErrRunNotFound.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
		Stability: stable,
	}, func(service interface {
		GetRun(context.Context, protocol.GetRunRequest) (*protocol.RunRef, error)
	}, ctx context.Context, request protocol.GetRunRequest) (*protocol.RunRef, error) {
		return service.GetRun(ctx, request)
	})

	// Only a request that asks for descendants needs features.subagents; the
	// default page of root runs is always available. The condition treats
	// `includeDescendants: false` as "not asking", so an explicit false and an
	// absent field behave alike — while an explicit true is refused rather than
	// read as false, which would hand back a page that looks complete and is not.
	Query(registry, MethodMeta{
		Name: "runs.list",
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
		Stability: stable,
	}, func(service interface {
		ListRuns(context.Context, protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error)
	}, ctx context.Context, request protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
		return service.ListRuns(ctx, request)
	})
}
