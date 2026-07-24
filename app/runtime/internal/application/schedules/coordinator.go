// Package schedules owns cron-triggered headless-run management and firing.
// Management is independent from execution; firing is built after Runs without
// mutable post-construction wiring.
package schedules

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// ManagementStore is the editable-schedule persistence slice owned by this
// use case. Firing and worker cursor updates intentionally remain separate.
type ManagementStore interface {
	List(ctx context.Context) ([]schedule.Schedule, error)
	Get(ctx context.Context, id string) (schedule.Schedule, error)
	Create(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error)
	Update(ctx context.Context, sc schedule.Schedule, expectedRevision uint64) (schedule.Schedule, error)
	Delete(ctx context.Context, id string) error
}

// Coordinator owns editable scheduled-run management over its narrow store.
// It is stateless beyond its dependencies and safe to share.
type Coordinator struct {
	registry ManagementStore
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
	Store ManagementStore
	Paths CwdResolver
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

// New returns a Coordinator over deps. A nil store yields a disabled
// coordinator (every CRUD operation returns [schedule.ErrUnavailable]).
func New(deps Dependencies) *Coordinator {
	registry := deps.Store
	enabled := registry != nil
	if registry == nil {
		registry = disabledManagementStore{}
	}
	return &Coordinator{
		registry: registry,
		paths:    deps.Paths,
		now:      time.Now,
		enabled:  enabled,
	}
}

// Available reports whether schedule-management use cases are wired.
func (c *Coordinator) Available() bool { return c != nil && c.enabled }

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

// disabledManagementStore is the no-scheduling CRUD fallback.
type disabledManagementStore struct{}

func (disabledManagementStore) List(context.Context) ([]schedule.Schedule, error) {
	return nil, schedule.ErrUnavailable
}

func (disabledManagementStore) Get(context.Context, string) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledManagementStore) Create(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledManagementStore) Update(context.Context, schedule.Schedule, uint64) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrUnavailable
}

func (disabledManagementStore) Delete(context.Context, string) error {
	return schedule.ErrUnavailable
}
