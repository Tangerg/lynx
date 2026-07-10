package runtime

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type disabledScheduleRegistry struct{}

func (disabledScheduleRegistry) List(context.Context) ([]schedule.Schedule, error) {
	return nil, schedule.ErrUnavailable
}

func (disabledScheduleRegistry) Get(context.Context, string) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledScheduleRegistry) Create(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledScheduleRegistry) Update(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledScheduleRegistry) Delete(context.Context, string) error {
	return schedule.ErrUnavailable
}

func (disabledScheduleRegistry) Due(context.Context, time.Time) ([]schedule.Schedule, error) {
	return nil, nil
}

func (disabledScheduleRegistry) MarkFired(context.Context, string, time.Time, time.Time, time.Time) error {
	return schedule.ErrUnavailable
}

func (disabledScheduleRegistry) RecordRun(context.Context, string, time.Time) error {
	return schedule.ErrUnavailable
}

// ListSchedules returns every saved schedule, newest-created first.
func (r *Runtime) ListSchedules(ctx context.Context) ([]schedule.Schedule, error) {
	return r.schedules.List(ctx)
}

// Schedule returns one saved schedule by id.
func (r *Runtime) Schedule(ctx context.Context, id string) (schedule.Schedule, error) {
	return r.schedules.Get(ctx, id)
}

// CreateSchedule persists a new schedule.
func (r *Runtime) CreateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	return r.schedules.Create(ctx, sc)
}

// UpdateSchedule full-replaces an existing schedule.
func (r *Runtime) UpdateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	return r.schedules.Update(ctx, sc)
}

// DeleteSchedule removes a schedule by id.
func (r *Runtime) DeleteSchedule(ctx context.Context, id string) error {
	return r.schedules.Delete(ctx, id)
}

// RecordScheduleRun records an off-cycle schedule firing without advancing the
// cron cursor.
func (r *Runtime) RecordScheduleRun(ctx context.Context, id string, ranAt time.Time) error {
	return r.schedules.RecordRun(ctx, id, ranAt)
}

// RunScheduleWorker starts the due-schedule scanner until ctx is canceled.
func (r *Runtime) RunScheduleWorker(ctx context.Context, runner schedule.Runner) {
	if r.scheduleWorker == nil {
		return
	}
	schedule.NewWorker(r.scheduleWorker, runner).Run(ctx)
}
