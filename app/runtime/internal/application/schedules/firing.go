package schedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/internal/domain/schedule"
)

// RunNowStore is the on-demand firing persistence slice.
type RunNowStore interface {
	Get(ctx context.Context, id string) (schedule.Schedule, error)
	RecordRun(ctx context.Context, id string, ranAt time.Time) error
}

// FiringStore joins the independently consumed run-now and worker slices for
// wiring one persistence implementation into this application component.
type FiringStore interface {
	RunNowStore
	WorkerStore
}

// Firing owns schedule execution after a management operation or worker tick.
// It is constructed with a complete ScheduledRunStarter, so callers cannot observe an
// incompletely wired scheduler.
type Firing struct {
	runNowStore   RunNowStore
	workerStore   WorkerStore
	runStarter    ScheduledRunStarter
	now           func() time.Time
	invalidations invalidation.Publish
}

// NewFiring builds the schedule execution use case. A nil store behaves as
// the unavailable scheduling capability.
func NewFiring(store FiringStore, runStarter ScheduledRunStarter, invalidations invalidation.Publish) *Firing {
	return &Firing{
		runNowStore: store, workerStore: store, runStarter: runStarter,
		now: time.Now, invalidations: invalidations,
	}
}

// Available reports whether schedule-firing use cases are wired.
func (f *Firing) Available() bool {
	return f != nil && f.runNowStore != nil && f.workerStore != nil
}

// RunNow starts one off-cycle schedule firing and records it without advancing
// the cron cursor. Once accepted, recording outlives request cancellation so a
// durable LastRunAt fact cannot be lost after a client disconnect.
func (f *Firing) RunNow(ctx context.Context, id string) (StartedRun, error) {
	if !f.Available() {
		return StartedRun{}, ErrUnavailable
	}
	scheduled, err := f.runNowStore.Get(ctx, id)
	if err != nil {
		return StartedRun{}, err
	}
	startedRun, err := Fire(ctx, f.runStarter, schedule.Occurrence{Schedule: scheduled})
	if err != nil {
		return StartedRun{}, err
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), manualRunRecordTimeout)
	defer cancel()
	if err := f.runNowStore.RecordRun(writeCtx, id, f.now().UTC()); err != nil {
		return StartedRun{}, fmt.Errorf("schedules: record run-now for %q: %w", id, err)
	}
	f.invalidations.Notify(invalidation.ForSchedules(id))
	return startedRun, nil
}

// RunWorker starts the due-schedule scanner until ctx is canceled.
func (f *Firing) RunWorker(ctx context.Context) {
	NewWorker(f.workerStore, f.runStarter, f.invalidations).Run(ctx)
}
