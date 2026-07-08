package runtime

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type scheduleList interface {
	List(ctx context.Context) ([]schedule.Schedule, error)
}

type scheduleRead interface {
	Get(ctx context.Context, id string) (schedule.Schedule, error)
}

type scheduleCreate interface {
	Create(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
}

type scheduleUpdate interface {
	Update(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
}

type scheduleDelete interface {
	Delete(ctx context.Context, id string) error
}

type scheduleRunRecorder interface {
	RecordRun(ctx context.Context, id string, ranAt time.Time) error
}

// ListSchedules returns every saved schedule, newest-created first.
func (r *Runtime) ListSchedules(ctx context.Context) ([]schedule.Schedule, error) {
	return r.scheduleList.List(ctx)
}

// Schedule returns one saved schedule by id.
func (r *Runtime) Schedule(ctx context.Context, id string) (schedule.Schedule, error) {
	return r.scheduleRead.Get(ctx, id)
}

// CreateSchedule persists a new schedule.
func (r *Runtime) CreateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	return r.scheduleCreation.Create(ctx, sc)
}

// UpdateSchedule full-replaces an existing schedule.
func (r *Runtime) UpdateSchedule(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	return r.scheduleUpdates.Update(ctx, sc)
}

// DeleteSchedule removes a schedule by id.
func (r *Runtime) DeleteSchedule(ctx context.Context, id string) error {
	return r.scheduleDeletion.Delete(ctx, id)
}

// RecordScheduleRun records an off-cycle schedule firing without advancing the
// cron cursor.
func (r *Runtime) RecordScheduleRun(ctx context.Context, id string, ranAt time.Time) error {
	return r.scheduleRuns.RecordRun(ctx, id, ranAt)
}

// RunScheduleWorker starts the due-schedule scanner until ctx is canceled.
func (r *Runtime) RunScheduleWorker(ctx context.Context, runner schedule.Runner) {
	schedule.NewWorker(r.scheduleWorker, runner).Run(ctx)
}
