// Package schedules owns cron-triggered headless-run management and firing.
// Management is independent from execution; firing is built after Runs without
// mutable post-construction wiring.
package schedules

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/pagination"
)

// ManagementStore is the editable-schedule persistence slice owned by this
// use case. Firing and worker cursor updates intentionally remain separate.
type ManagementStore interface {
	List(ctx context.Context) ([]schedule.Schedule, error)
	ListPage(ctx context.Context, afterCreatedAt int64, afterID string, limit int) ([]schedule.Schedule, error)
	Get(ctx context.Context, id string) (schedule.Schedule, error)
	Create(ctx context.Context, scheduled schedule.Schedule) (schedule.Schedule, error)
	Update(ctx context.Context, scheduled schedule.Schedule, expectedRevision uint64) (schedule.Schedule, error)
	Delete(ctx context.Context, id string) error
}

// Coordinator owns editable scheduled-run management over its narrow store.
// It is stateless beyond its dependencies and safe to share.
type Coordinator struct {
	store   ManagementStore
	paths   CWDResolver
	now     func() time.Time
	enabled bool
}

// CWDResolver is the filesystem boundary used to admit a schedule's working
// directory. Persisted schedules always hold either an empty cwd (the runtime
// default) or a canonical existing directory.
type CWDResolver interface {
	ResolveExistingDir(path string) (string, error)
}

// Dependencies is the collaborator set [New] wires into a Coordinator.
type Dependencies struct {
	Store ManagementStore
	Paths CWDResolver
}

// CreateCommand is the complete editable state of a new schedule.
type CreateCommand struct {
	Title          string
	Instructions   string
	CWD            string
	ModelSelection modelref.Selection
	Cron           string
	Enabled        bool
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
	store := deps.Store
	enabled := store != nil
	if store == nil {
		store = disabledManagementStore{}
	}
	return &Coordinator{
		store:   store,
		paths:   deps.Paths,
		now:     time.Now,
		enabled: enabled,
	}
}

// Available reports whether schedule-management use cases are wired.
func (c *Coordinator) Available() bool { return c != nil && c.enabled }

// List returns every saved schedule, newest-created first.
func (c *Coordinator) List(ctx context.Context) ([]schedule.Schedule, error) {
	return c.store.List(ctx)
}

// listPageNamespace binds cursors to this schedule read independently of other
// paged reads.
const listPageNamespace = "schedules"

// listPageLimit is the widest schedule page this read will serve.
const listPageLimit = 100

// ListPage returns one page of schedules, newest-created first, continuing after
// cursor.
func (c *Coordinator) ListPage(ctx context.Context, cursor string, limit int) (pagination.Page[schedule.Schedule], error) {
	anchor, err := pagination.Decode(cursor, listPageNamespace, nil)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	var afterCreatedAt int64
	var afterID string
	if len(anchor) > 0 {
		if len(anchor) != 2 {
			return pagination.Page[schedule.Schedule]{}, pagination.ErrInvalidCursor
		}
		if afterCreatedAt, err = strconv.ParseInt(anchor[0], 10, 64); err != nil {
			return pagination.Page[schedule.Schedule]{}, pagination.ErrInvalidCursor
		}
		afterID = anchor[1]
	}
	size, err := pagination.Limit(limit, listPageLimit)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	rows, err := c.store.ListPage(ctx, afterCreatedAt, afterID, size+1)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	return pagination.PageOf(rows, size, listPageNamespace, nil, func(scheduled schedule.Schedule) []string {
		return []string{strconv.FormatInt(scheduled.CreatedAt.UnixNano(), 10), scheduled.ID}
	}), nil
}

// Create validates, normalizes, schedules, and persists a new schedule.
func (c *Coordinator) Create(ctx context.Context, cmd CreateCommand) (schedule.Schedule, error) {
	if !c.enabled {
		return schedule.Schedule{}, schedule.ErrUnavailable
	}
	scheduled, err := (schedule.Schedule{
		Title:          cmd.Title,
		Instructions:   cmd.Instructions,
		CWD:            cmd.CWD,
		ModelSelection: cmd.ModelSelection,
		Cron:           cmd.Cron,
		Enabled:        cmd.Enabled,
	}).ScheduledAfter(c.now())
	if err != nil {
		return schedule.Schedule{}, err
	}
	scheduled.CWD, err = c.resolveCWD(scheduled.CWD)
	if err != nil {
		return schedule.Schedule{}, err
	}
	created, err := c.store.Create(ctx, scheduled)
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
	existing, err := c.store.Get(ctx, cmd.ID)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("schedules: get %q for update: %w", cmd.ID, err)
	}
	return c.updateExisting(ctx, existing, cmd.Patch, cmd.ExpectedRevision)
}

func (c *Coordinator) updateExisting(
	ctx context.Context,
	existing schedule.Schedule,
	patch schedule.Patch,
	expectedRevision uint64,
) (schedule.Schedule, error) {
	updated, err := existing.Apply(patch)
	if err != nil {
		return schedule.Schedule{}, err
	}
	updated, err = updated.ScheduledAfter(c.now())
	if err != nil {
		return schedule.Schedule{}, err
	}
	if patch.CWD != nil {
		updated.CWD, err = c.resolveCWD(updated.CWD)
		if err != nil {
			return schedule.Schedule{}, err
		}
	}
	updated, err = c.store.Update(ctx, updated, expectedRevision)
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
	if err := c.store.Delete(ctx, id); err != nil {
		return fmt.Errorf("schedules: delete %q: %w", id, err)
	}
	return nil
}

func (c *Coordinator) resolveCWD(cwd string) (string, error) {
	if cwd == "" {
		return "", nil
	}
	if c.paths == nil {
		return "", errors.Join(schedule.ErrCWDUnavailable, errors.New("schedules: cwd resolver is unavailable"))
	}
	resolved, err := c.paths.ResolveExistingDir(cwd)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %w", schedule.ErrCWDUnavailable, cwd, err)
	}
	return resolved, nil
}

// disabledManagementStore is the no-scheduling CRUD fallback.
type disabledManagementStore struct{}

func (disabledManagementStore) List(context.Context) ([]schedule.Schedule, error) {
	return nil, schedule.ErrUnavailable
}

func (disabledManagementStore) ListPage(context.Context, int64, string, int) ([]schedule.Schedule, error) {
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
