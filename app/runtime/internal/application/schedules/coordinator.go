// Package schedules is the application coordinator for cron-triggered headless
// runs: CRUD over saved schedules, off-cycle firing, and the background
// due-schedule worker. It is a thin use-case layer over the domain schedule
// registry + worker store — the delivery layer drives it and supplies the
// Runner that turns a fired schedule into a run.
package schedules

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// Coordinator owns the scheduled-run use cases over the domain registry. It is
// stateless beyond its dependencies and safe to share.
type Coordinator struct {
	registry schedule.Registry
	worker   WorkerStore
	paths    CwdResolver
	now      func() time.Time
	enabled  bool
}

// CwdResolver is the filesystem boundary used to admit a schedule's working
// directory. Persisted schedules always hold either an empty cwd (the runtime
// default) or a canonical existing directory.
type CwdResolver interface {
	ResolveExistingDir(path string) (string, error)
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Registry schedule.Registry
	Worker   WorkerStore
	Paths    CwdResolver
}

// CreateCommand is the complete editable state of a new schedule.
type CreateCommand struct {
	Title    string
	Prompt   string
	Cwd      string
	Provider string
	Model    string
	Cron     string
	Enabled  bool
}

// UpdateCommand applies a partial edit to one stored schedule.
type UpdateCommand struct {
	ID               string
	Patch            schedule.Patch
	ExpectedRevision uint64
}

// New returns a Coordinator over deps. A nil registry yields a disabled
// coordinator (every CRUD operation returns [schedule.ErrUnavailable]); a nil
// worker disables the background scanner.
func New(deps Dependencies) *Coordinator {
	registry := deps.Registry
	enabled := registry != nil
	if registry == nil {
		registry = disabledRegistry{}
	}
	return &Coordinator{
		registry: registry,
		worker:   deps.Worker,
		paths:    deps.Paths,
		now:      time.Now,
		enabled:  enabled,
	}
}

// List returns every saved schedule, newest-created first.
func (c *Coordinator) List(ctx context.Context) ([]schedule.Schedule, error) {
	return c.registry.List(ctx)
}

// Get returns one saved schedule by id.
func (c *Coordinator) Get(ctx context.Context, id string) (schedule.Schedule, error) {
	return c.registry.Get(ctx, id)
}

// Create validates, normalizes, schedules, and persists a new schedule.
func (c *Coordinator) Create(ctx context.Context, cmd CreateCommand) (schedule.Schedule, error) {
	if !c.enabled {
		return schedule.Schedule{}, schedule.ErrUnavailable
	}
	sc, err := (schedule.Schedule{
		Title:    cmd.Title,
		Prompt:   cmd.Prompt,
		Cwd:      cmd.Cwd,
		Provider: cmd.Provider,
		Model:    cmd.Model,
		Cron:     cmd.Cron,
		Enabled:  cmd.Enabled,
	}).ScheduledAfter(c.now())
	if err != nil {
		return schedule.Schedule{}, err
	}
	sc.Cwd, err = c.resolveCwd(sc.Cwd)
	if err != nil {
		return schedule.Schedule{}, err
	}
	created, err := c.registry.Create(ctx, sc)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: create: %w", err)
	}
	return created, nil
}

// Update applies a patch to an existing schedule, preserving durable identity
// and timestamps while recomputing its next due time.
func (c *Coordinator) Update(ctx context.Context, cmd UpdateCommand) (schedule.Schedule, error) {
	if !c.enabled {
		return schedule.Schedule{}, schedule.ErrUnavailable
	}
	if cmd.ID == "" {
		return schedule.Schedule{}, schedule.ErrIDRequired
	}
	if cmd.ExpectedRevision == 0 {
		return schedule.Schedule{}, schedule.ErrRevisionRequired
	}
	existing, err := c.registry.Get(ctx, cmd.ID)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: get %q for update: %w", cmd.ID, err)
	}
	return c.updateExisting(ctx, existing, cmd.Patch, cmd.ExpectedRevision)
}

// UpdateLatest applies an internal automation patch to the latest durable
// revision. Unlike the user-facing Update command, an Agent tool has no stale UI
// snapshot to protect; the coordinator's own read is its OCC baseline.
func (c *Coordinator) UpdateLatest(ctx context.Context, id string, patch schedule.Patch) (schedule.Schedule, error) {
	if !c.enabled {
		return schedule.Schedule{}, schedule.ErrUnavailable
	}
	if id == "" {
		return schedule.Schedule{}, schedule.ErrIDRequired
	}
	existing, err := c.registry.Get(ctx, id)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: get %q for update: %w", id, err)
	}
	return c.updateExisting(ctx, existing, patch, existing.Revision)
}

func (c *Coordinator) updateExisting(
	ctx context.Context,
	existing schedule.Schedule,
	patch schedule.Patch,
	expectedRevision uint64,
) (schedule.Schedule, error) {
	updated, err := existing.Apply(patch).ScheduledAfter(c.now())
	if err != nil {
		return schedule.Schedule{}, err
	}
	if patch.Cwd != nil {
		updated.Cwd, err = c.resolveCwd(updated.Cwd)
		if err != nil {
			return schedule.Schedule{}, err
		}
	}
	updated, err = c.registry.Update(ctx, updated, expectedRevision)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: update %q: %w", existing.ID, err)
	}
	return updated, nil
}

// Delete removes a schedule by id.
func (c *Coordinator) Delete(ctx context.Context, id string) error {
	if !c.enabled {
		return schedule.ErrUnavailable
	}
	if id == "" {
		return schedule.ErrIDRequired
	}
	if err := c.registry.Delete(ctx, id); err != nil {
		return fmt.Errorf("schedules: delete %q: %w", id, err)
	}
	return nil
}

func (c *Coordinator) resolveCwd(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	if c.paths == nil {
		return "", errors.Join(schedule.ErrCwdUnavailable, errors.New("schedules: cwd resolver is unavailable"))
	}
	resolved, err := c.paths.ResolveExistingDir(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %w", schedule.ErrCwdUnavailable, cwd, err)
	}
	return resolved, nil
}

// RunNow starts one off-cycle schedule firing and records it without advancing
// the cron cursor. Once the run is accepted, recording uses a bounded context
// detached from the request: a client disconnect must not make the durable
// LastRunAt claim that the accepted run never happened.
func (c *Coordinator) RunNow(ctx context.Context, id string, runner Runner) (RunHandle, error) {
	sc, err := c.registry.Get(ctx, id)
	if err != nil {
		return RunHandle{}, err
	}
	handle, err := Fire(ctx, runner, sc)
	if err != nil {
		return RunHandle{}, err
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), firingStateWriteTimeout)
	defer cancel()
	if err := c.registry.RecordRun(writeCtx, id, c.now().UTC()); err != nil {
		return RunHandle{}, fmt.Errorf("schedules: record run-now for %q: %w", id, err)
	}
	return handle, nil
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

func (disabledRegistry) Update(context.Context, schedule.Schedule, uint64) (schedule.Schedule, error) {
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
