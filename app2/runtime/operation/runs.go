package operation

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app2/runtime/protocol"
)

func registerRuns(registry *Registry) {
	RunStreamCommand(registry, MethodMeta{
		Name: "runs.start",
		Errors: []string{
			protocol.ErrSessionNotFound.Error(), protocol.ErrSessionBusy.Error(),
			protocol.ErrSessionHasActiveRun.Error(), protocol.ErrUnsupportedMime.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
	}, func(service interface {
		StartRun(context.Context, protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error)
	}, ctx context.Context, request protocol.StartRunRequest) (*protocol.StartRunResponse, iter.Seq[protocol.RunEvent], error) {
		return service.StartRun(ctx, request)
	})

	RunStreamCommand(registry, MethodMeta{
		Name: "runs.resume",
		Errors: []string{
			protocol.ErrRunNotFound.Error(), protocol.ErrInterruptNotOpen.Error(),
			protocol.ErrCapabilityNotNeg.Error(),
		},
	}, func(service interface {
		ResumeRun(context.Context, protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error)
	}, ctx context.Context, request protocol.ResumeRunRequest) (*protocol.ResumeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return service.ResumeRun(ctx, request)
	})

	Subscription(registry, MethodMeta{
		Name: "runs.subscribe",
		Errors: []string{
			protocol.ErrRunNotFound.Error(), protocol.ErrRunNotRoot.Error(),
			protocol.ErrRunWaiting.Error(), protocol.ErrRunFinished.Error(),
			protocol.ErrStaleSegment.Error(), protocol.ErrReplayCursorInvalid.Error(),
			protocol.ErrReplayUnavailable.Error(), protocol.ErrCapabilityNotNeg.Error(),
		},
	}, func(service interface {
		SubscribeRun(context.Context, protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error)
	}, ctx context.Context, request protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq[protocol.RunEvent], error) {
		return service.SubscribeRun(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "runs.cancel",
		Errors: []string{
			protocol.ErrRunNotFound.Error(), protocol.ErrRunFinished.Error(),
			protocol.ErrSessionBusy.Error(), protocol.ErrCapabilityNotNeg.Error(),
		},
	}, func(service interface {
		CancelRun(context.Context, protocol.CancelRunRequest) (*protocol.CancelRunResponse, error)
	}, ctx context.Context, request protocol.CancelRunRequest) (*protocol.CancelRunResponse, error) {
		return service.CancelRun(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "runs.steer",
		Errors: []string{
			protocol.ErrRunNotFound.Error(), protocol.ErrRunNotRoot.Error(),
			protocol.ErrRunWaiting.Error(), protocol.ErrRunFinished.Error(),
			protocol.ErrStaleSegment.Error(),
		},
	}, func(service interface {
		SteerRun(context.Context, protocol.SteerRunRequest) error
	}, ctx context.Context, request protocol.SteerRunRequest) error {
		return service.SteerRun(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "runs.get",
		Errors: []string{protocol.ErrRunNotFound.Error(), protocol.ErrCapabilityNotNeg.Error()},
	}, func(service interface {
		GetRun(context.Context, protocol.GetRunRequest) (*protocol.RunRef, error)
	}, ctx context.Context, request protocol.GetRunRequest) (*protocol.RunRef, error) {
		return service.GetRun(ctx, request)
	})

	Query(registry, MethodMeta{
		Name: "runs.list",
		CapabilityRules: []CapabilityRule{{
			When: []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
	}, func(service interface {
		ListRuns(context.Context, protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error)
	}, ctx context.Context, request protocol.ListRunsRequest) (*protocol.Page[protocol.RunRef], error) {
		return service.ListRuns(ctx, request)
	})
}
