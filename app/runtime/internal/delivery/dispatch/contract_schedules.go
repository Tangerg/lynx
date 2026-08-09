package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerSchedules(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "schedules.list", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
		return router.api.ListSchedules(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "schedules.create", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
		return router.api.CreateSchedule(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "schedules.update", Errors: []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return router.api.UpdateSchedule(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "schedules.delete", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.DeleteScheduleRequest) error {
		return router.api.DeleteSchedule(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "schedules.runNow", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(router *Router, ctx context.Context, request protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
		return router.api.RunScheduleNow(ctx, request)
	})
}
