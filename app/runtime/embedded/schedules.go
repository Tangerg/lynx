package embedded

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/operation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// ListSchedules returns one cursor page of schedules.
func (r *Runtime) ListSchedules(ctx context.Context, request protocol.PageQuery, options CallOptions) (*protocol.Page[protocol.Schedule], error) {
	return r.invoke[protocol.PageQuery, *protocol.Page[protocol.Schedule]](ctx, operation.SchedulesList, request, callOptions(options))
}

// CreateSchedule creates a schedule.
func (r *Runtime) CreateSchedule(ctx context.Context, request protocol.CreateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return r.invoke[protocol.CreateScheduleRequest, *protocol.Schedule](ctx, operation.SchedulesCreate, request, commandOptions(options))
}

// UpdateSchedule applies a revision-checked schedule edit.
func (r *Runtime) UpdateSchedule(ctx context.Context, request protocol.UpdateScheduleRequest, options CommandOptions) (*protocol.Schedule, error) {
	return r.invoke[protocol.UpdateScheduleRequest, *protocol.Schedule](ctx, operation.SchedulesUpdate, request, commandOptions(options))
}

// DeleteSchedule deletes a schedule.
func (r *Runtime) DeleteSchedule(ctx context.Context, request protocol.DeleteScheduleRequest, options CommandOptions) error {
	return r.invokeAck(ctx, operation.SchedulesDelete, request, commandOptions(options))
}

// RunScheduleNow fires a schedule without advancing its cron cursor.
func (r *Runtime) RunScheduleNow(ctx context.Context, request protocol.RunScheduleNowRequest, options CommandOptions) (*protocol.RunScheduleNowResponse, error) {
	return r.invoke[protocol.RunScheduleNowRequest, *protocol.RunScheduleNowResponse](ctx, operation.SchedulesRunNow, request, commandOptions(options))
}
