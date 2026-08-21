package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func registerSchedules(registry *Registry) {
	Query(registry, MethodMeta{
		Name: "schedules.list", CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		ListSchedules(context.Context, protocol.PageQuery) (*protocol.Page[protocol.Schedule], error)
	}, ctx context.Context, request protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
		return service.ListSchedules(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "schedules.create", CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		CreateSchedule(context.Context, protocol.CreateScheduleRequest) (*protocol.Schedule, error)
	}, ctx context.Context, request protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
		return service.CreateSchedule(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "schedules.update", Errors: []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		UpdateSchedule(context.Context, protocol.UpdateScheduleRequest) (*protocol.Schedule, error)
	}, ctx context.Context, request protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return service.UpdateSchedule(ctx, request)
	})

	CommandAck(registry, MethodMeta{
		Name: "schedules.delete", CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		DeleteSchedule(context.Context, protocol.DeleteScheduleRequest) error
	}, ctx context.Context, request protocol.DeleteScheduleRequest) error {
		return service.DeleteSchedule(ctx, request)
	})

	Command(registry, MethodMeta{
		Name: "schedules.runNow", CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		RunScheduleNow(context.Context, protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error)
	}, ctx context.Context, request protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
		return service.RunScheduleNow(ctx, request)
	})
}
