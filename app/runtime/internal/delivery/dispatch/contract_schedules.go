package dispatch

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func registerSchedules(r *Registry) {
	Query(r, MethodMeta{
		Name: "schedules.list", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
		return d.api.ListSchedules(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "schedules.create", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
		return d.api.CreateSchedule(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "schedules.update", Errors: []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return d.api.UpdateSchedule(ctx, in)
	})

	CommandAck(r, MethodMeta{
		Name: "schedules.delete", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.DeleteScheduleRequest) error {
		return d.api.DeleteSchedule(ctx, in)
	})

	Command(r, MethodMeta{
		Name: "schedules.runNow", CapabilityRules: requires(protocol.FeatureSchedules), Stability: stable,
	}, func(d *Router, ctx context.Context, in protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
		return d.api.RunScheduleNow(ctx, in)
	})
}
