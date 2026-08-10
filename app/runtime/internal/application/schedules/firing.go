package schedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
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
	runNowStore RunNowStore
	workerStore WorkerStore
	runStarter  ScheduledRunStarter
	now         func() time.Time
	enabled     bool
}

// NewFiring builds the schedule execution use case. A nil store behaves as
// the unavailable scheduling capability.
func NewFiring(store FiringStore, runStarter ScheduledRunStarter) *Firing {
	enabled := store != nil
	if store == nil {
		store = disabledFiringStore{}
	}
	return &Firing{runNowStore: store, workerStore: store, runStarter: runStarter, now: time.Now, enabled: enabled}
}

// Available reports whether schedule-firing use cases are wired.
func (f *Firing) Available() bool { return f != nil && f.enabled }

// RunNow starts one off-cycle schedule firing and records it without advancing
// the cron cursor. Once accepted, recording outlives request cancellation so a
// durable LastRunAt fact cannot be lost after a client disconnect.
func (f *Firing) RunNow(ctx context.Context, id string) (StartedRun, error) {
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
	return startedRun, nil
}

// RunWorker starts the due-schedule scanner until ctx is canceled.
func (f *Firing) RunWorker(ctx context.Context) {
	NewWorker(f.workerStore, f.runStarter).Run(ctx)
}

type disabledFiringStore struct{}

func (disabledFiringStore) Get(context.Context, string) (schedule.Schedule, error) {
	return schedule.Schedule{}, ErrUnavailable
}

func (disabledFiringStore) RecordRun(context.Context, string, time.Time) error {
	return ErrUnavailable
}

func (disabledFiringStore) Due(context.Context, time.Time, int) ([]schedule.Schedule, error) {
	return nil, nil
}

func (disabledFiringStore) Claim(context.Context, schedule.Occurrence) (bool, error) {
	return false, ErrUnavailable
}

func (disabledFiringStore) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return nil, ErrUnavailable
}
