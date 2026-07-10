// Package schedules is the application coordinator for cron-triggered headless
// runs: CRUD over saved schedules, off-cycle run recording, and the background
// due-schedule worker. It is a thin use-case layer over the domain schedule
// registry + worker store — the delivery layer drives it and supplies the
// Runner that turns a fired schedule into a run.
package schedules

import (
	"context"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// Coordinator owns the scheduled-run use cases over the domain registry. It is
// stateless beyond its dependencies and safe to share.
type Coordinator struct {
	registry schedule.Registry
	worker   schedule.WorkerStore
}

// NewCoordinator returns a Coordinator over the schedule registry. A nil
// registry yields a disabled coordinator (every CRUD op returns
// [schedule.ErrUnavailable]); a nil worker disables the background scanner. The
// same store is passed as both when scheduling is available.
func NewCoordinator(registry schedule.Registry, worker schedule.WorkerStore) *Coordinator {
	if registry == nil {
		registry = disabledRegistry{}
	}
	return &Coordinator{registry: registry, worker: worker}
}

// List returns every saved schedule, newest-created first.
func (c *Coordinator) List(ctx context.Context) ([]schedule.Schedule, error) {
	return c.registry.List(ctx)
}

// Get returns one saved schedule by id.
func (c *Coordinator) Get(ctx context.Context, id string) (schedule.Schedule, error) {
	return c.registry.Get(ctx, id)
}

// Create persists a new schedule.
func (c *Coordinator) Create(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	return c.registry.Create(ctx, sc)
}

// Update full-replaces an existing schedule.
func (c *Coordinator) Update(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	return c.registry.Update(ctx, sc)
}

// Delete removes a schedule by id.
func (c *Coordinator) Delete(ctx context.Context, id string) error {
	return c.registry.Delete(ctx, id)
}

// RecordRun records an off-cycle schedule firing without advancing the cron cursor.
func (c *Coordinator) RecordRun(ctx context.Context, id string, ranAt time.Time) error {
	return c.registry.RecordRun(ctx, id, ranAt)
}

// RunWorker starts the due-schedule scanner until ctx is canceled. No worker
// store → no-op (scheduling unavailable on this build).
func (c *Coordinator) RunWorker(ctx context.Context, runner schedule.Runner) {
	if c.worker == nil {
		return
	}
	schedule.NewWorker(c.worker, runner).Run(ctx)
}

// disabledRegistry is the no-scheduling fallback: CRUD reports unavailable while
// Due returns empty so the worker (if ever run) simply finds nothing to fire.
type disabledRegistry struct{}

func (disabledRegistry) List(context.Context) ([]schedule.Schedule, error) {
	return nil, schedule.ErrUnavailable
}

func (disabledRegistry) Get(context.Context, string) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledRegistry) Create(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledRegistry) Update(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledRegistry) Delete(context.Context, string) error {
	return schedule.ErrUnavailable
}

func (disabledRegistry) Due(context.Context, time.Time) ([]schedule.Schedule, error) {
	return nil, nil
}

func (disabledRegistry) MarkFired(context.Context, string, time.Time, time.Time, time.Time) error {
	return schedule.ErrUnavailable
}

func (disabledRegistry) RecordRun(context.Context, string, time.Time) error {
	return schedule.ErrUnavailable
}
