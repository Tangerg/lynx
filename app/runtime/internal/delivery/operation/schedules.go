package operation

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

const (
	SchedulesList   Name = "schedules.list"
	SchedulesCreate Name = "schedules.create"
	SchedulesUpdate Name = "schedules.update"
	SchedulesDelete Name = "schedules.delete"
	SchedulesRunNow Name = "schedules.runNow"
)

func registerSchedules(registry *Registry) {
	registry.Query(MethodMeta{
		Name: SchedulesList, CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		ListSchedules(context.Context, protocol.PageQuery) (*protocol.Page[protocol.Schedule], error)
	}, ctx context.Context, request protocol.PageQuery) (*protocol.Page[protocol.Schedule], error) {
		return service.ListSchedules(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: SchedulesCreate, CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		CreateSchedule(context.Context, protocol.CreateScheduleRequest) (*protocol.Schedule, error)
	}, ctx context.Context, request protocol.CreateScheduleRequest) (*protocol.Schedule, error) {
		return service.CreateSchedule(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: SchedulesUpdate, Errors: []string{protocol.ErrRevisionConflict.Error()},
		CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		UpdateSchedule(context.Context, protocol.UpdateScheduleRequest) (*protocol.Schedule, error)
	}, ctx context.Context, request protocol.UpdateScheduleRequest) (*protocol.Schedule, error) {
		return service.UpdateSchedule(ctx, request)
	})

	registry.CommandAck(MethodMeta{
		Name: SchedulesDelete, CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		DeleteSchedule(context.Context, protocol.DeleteScheduleRequest) error
	}, ctx context.Context, request protocol.DeleteScheduleRequest) error {
		return service.DeleteSchedule(ctx, request)
	})

	registry.Command(MethodMeta{
		Name: SchedulesRunNow, CapabilityRules: requires(protocol.FeatureSchedules),
	}, func(service interface {
		RunScheduleNow(context.Context, protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error)
	}, ctx context.Context, request protocol.RunScheduleNowRequest) (*protocol.RunScheduleNowResponse, error) {
		return service.RunScheduleNow(ctx, request)
	})
}
