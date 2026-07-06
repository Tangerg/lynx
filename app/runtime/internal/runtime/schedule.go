package runtime

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

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
	schedule.NewWorker(r.schedules, runner).Run(ctx)
}
