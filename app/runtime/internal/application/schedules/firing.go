package schedules

import (
	"context"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// Firing owns schedule execution after a management operation or worker tick.
// It is constructed with a complete Runner, so callers cannot observe an
// incompletely wired scheduler.
type Firing struct {
	registry schedule.Registry
	runner   Runner
	now      func() time.Time
}

// NewFiring builds the schedule execution use case. A nil registry behaves as
// the unavailable scheduling capability.
func NewFiring(registry schedule.Registry, runner Runner) *Firing {
	if registry == nil {
		registry = disabledRegistry{}
	}
	return &Firing{registry: registry, runner: runner, now: time.Now}
}

// RunNow starts one off-cycle schedule firing and records it without advancing
// the cron cursor. Once accepted, recording outlives request cancellation so a
// durable LastRunAt fact cannot be lost after a client disconnect.
func (f *Firing) RunNow(ctx context.Context, id string) (RunHandle, error) {
	sc, err := f.registry.Get(ctx, id)
	if err != nil {
		return RunHandle{}, err
	}
	handle, err := Fire(ctx, f.runner, sc)
	if err != nil {
		return RunHandle{}, err
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), firingStateWriteTimeout)
	defer cancel()
	if err := f.registry.RecordRun(writeCtx, id, f.now().UTC()); err != nil {
		return RunHandle{}, fmt.Errorf("schedules: record run-now for %q: %w", id, err)
	}
	return handle, nil
}

// RunWorker starts the due-schedule scanner until ctx is canceled.
func (f *Firing) RunWorker(ctx context.Context) {
	NewWorker(f.registry, f.runner).Run(ctx)
}
