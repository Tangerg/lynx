// Package schedules is the application coordinator for cron-triggered headless
// runs: CRUD over saved schedules, off-cycle firing, and the background
// due-schedule worker. It is a thin use-case layer over the domain schedule
// registry + worker store — the delivery layer drives it and supplies the
// Runner that turns a fired schedule into a run.
package schedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// Coordinator owns the scheduled-run use cases over the domain registry. It is
// stateless beyond its dependencies and safe to share.
type Coordinator struct {
	registry schedule.Registry
	worker   WorkerStore
	now      func() time.Time
}

// NewCoordinator returns a Coordinator over the schedule registry. A nil
// registry yields a disabled coordinator (every CRUD op returns
// [schedule.ErrUnavailable]); a nil worker disables the background scanner. The
// same store is passed as both when scheduling is available.
func NewCoordinator(registry schedule.Registry, worker WorkerStore) *Coordinator {
	if registry == nil {
		registry = disabledRegistry{}
	}
	return &Coordinator{registry: registry, worker: worker, now: time.Now}
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

// RunNow starts one off-cycle schedule firing and records it without advancing
// the cron cursor. Once the run is accepted, recording uses a bounded context
// detached from the request: a client disconnect must not make the durable
// LastRunAt claim that the accepted run never happened.
func (c *Coordinator) RunNow(ctx context.Context, id string, runner Runner) error {
	sc, err := c.registry.Get(ctx, id)
	if err != nil {
		return err
	}
	if _, err := Fire(ctx, runner, sc); err != nil {
		return err
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), firingStateWriteTimeout)
	defer cancel()
	if err := c.registry.RecordRun(writeCtx, id, c.now().UTC()); err != nil {
		return fmt.Errorf("schedules: record run-now for %q: %w", id, err)
	}
	return nil
}

// RunWorker starts the due-schedule scanner until ctx is canceled. No worker
// store → no-op (scheduling unavailable on this build).
func (c *Coordinator) RunWorker(ctx context.Context, runner Runner) {
	if c.worker == nil {
		return
	}
	NewWorker(c.worker, runner).Run(ctx)
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
