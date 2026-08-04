package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/keyset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// TestNilRegistryDisablesCRUD: a coordinator built without a registry reports
// every CRUD op as unavailable (the no-scheduling build), rather than panicking.
func TestNilRegistryDisablesCRUD(t *testing.T) {
	c := New(Dependencies{})
	ctx := context.Background()

	if _, err := c.List(ctx); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("List err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Create(ctx, CreateCommand{}); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("Create err = %v, want ErrUnavailable", err)
	}
	if err := c.Delete(ctx, "sch_1"); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("Delete err = %v, want ErrUnavailable", err)
	}
	if _, err := NewFiring(nil, nil).RunNow(ctx, "sch_1"); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("RunNow err = %v, want ErrUnavailable", err)
	}
}

func TestRunNowRecordsAcceptedRunAfterRequestCancellation(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	registry := &runNowRegistry{schedule: schedule.Schedule{ID: "sch_1", Prompt: "review"}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingWorkerRunner{cancel: cancel, succeed: true}
	firing := NewFiring(registry, &runner)
	firing.now = func() time.Time { return now }

	if _, err := firing.RunNow(ctx, "sch_1"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if registry.recordedID != "sch_1" || !registry.recordedAt.Equal(now) {
		t.Fatalf("recorded = (%q, %v), want (sch_1, %v)", registry.recordedID, registry.recordedAt, now)
	}
	if registry.recordCtxErr != nil {
		t.Fatalf("record context error = %v, want live post-accept context", registry.recordCtxErr)
	}
}

func TestRunNowDoesNotRecordCancellationAbortedRun(t *testing.T) {
	registry := &runNowRegistry{schedule: schedule.Schedule{ID: "sch_1", Prompt: "review"}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingWorkerRunner{cancel: cancel, succeed: false}
	firing := NewFiring(registry, &runner)

	if _, err := firing.RunNow(ctx, "sch_1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunNow error = %v, want context.Canceled", err)
	}
	if registry.recordedID != "" {
		t.Fatalf("recorded id = %q, want none", registry.recordedID)
	}
}

type cwdResolverFunc func(string) (string, error)

func (f cwdResolverFunc) ResolveExistingDir(path string) (string, error) {
	return f(path)
}

func TestCreateOwnsScheduleAdmission(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	registry := &runNowRegistry{}
	c := New(Dependencies{
		Store: registry,
		Paths: cwdResolverFunc(func(path string) (string, error) {
			if path != "workspace" {
				t.Fatalf("ResolveExistingDir(%q), want workspace", path)
			}
			return "/canonical/workspace", nil
		}),
	})
	c.now = func() time.Time { return now }

	created, err := c.Create(t.Context(), CreateCommand{
		Prompt:  "review",
		Cwd:     "workspace",
		Cron:    "0 13 * * *",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Cwd != "/canonical/workspace" || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}
	wantNext, err := schedule.NextRun("0 13 * * *", now)
	if err != nil {
		t.Fatalf("NextRun: %v", err)
	}
	if !created.NextRunAt.Equal(wantNext) {
		t.Fatalf("NextRunAt = %v, want %v", created.NextRunAt, wantNext)
	}
}

func TestUpdateOwnsPatchAndPreservesSnapshotState(t *testing.T) {
	lastRun := time.Date(2026, 7, 18, 9, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	registry := &runNowRegistry{schedule: schedule.Schedule{
		ID:        "sch_1",
		Prompt:    "before",
		Cwd:       "/before",
		Cron:      "0 9 * * *",
		Enabled:   true,
		LastRunAt: lastRun,
		CreatedAt: createdAt,
	}}
	c := New(Dependencies{
		Store: registry,
		Paths: cwdResolverFunc(func(string) (string, error) {
			return "/canonical/after", nil
		}),
	})
	cwd, prompt, enabled := "after", "after", false

	updated, err := c.UpdateLatest(t.Context(), "sch_1", schedule.Patch{
		Prompt:  &prompt,
		Cwd:     &cwd,
		Enabled: &enabled,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID != "sch_1" || updated.Prompt != "after" || updated.Cwd != "/canonical/after" {
		t.Fatalf("updated = %+v", updated)
	}
	if !updated.LastRunAt.Equal(lastRun) || !updated.CreatedAt.Equal(createdAt) || !updated.NextRunAt.IsZero() {
		t.Fatalf("updated durable state = %+v", updated)
	}
}

func TestUpdateRequiresAnExplicitRevision(t *testing.T) {
	c := New(Dependencies{Store: &runNowRegistry{schedule: schedule.Schedule{ID: "sch_1"}}})
	_, err := c.Update(t.Context(), UpdateCommand{ID: "sch_1"})
	if !errors.Is(err, schedule.ErrRevisionRequired) {
		t.Fatalf("Update error = %v, want ErrRevisionRequired", err)
	}
}

func TestCreateValidatesBeforeResolvingCwd(t *testing.T) {
	resolved := false
	c := New(Dependencies{
		Store: &runNowRegistry{},
		Paths: cwdResolverFunc(func(string) (string, error) {
			resolved = true
			return "", errors.New("unexpected resolution")
		}),
	})
	_, err := c.Create(t.Context(), CreateCommand{Cwd: "missing", Cron: "@daily", Enabled: true})
	if !errors.Is(err, schedule.ErrPromptRequired) {
		t.Fatalf("Create error = %v, want ErrPromptRequired", err)
	}
	if resolved {
		t.Fatal("cwd was resolved before schedule validation")
	}
}

type runNowRegistry struct {
	schedule     schedule.Schedule
	created      schedule.Schedule
	updated      schedule.Schedule
	recordedID   string
	recordedAt   time.Time
	recordCtxErr error
}

func (r *runNowRegistry) ListPage(ctx context.Context, _ int64, _ string, _ int) ([]schedule.Schedule, error) {
	return r.List(ctx)
}

func (r *runNowRegistry) List(context.Context) ([]schedule.Schedule, error) { return nil, nil }
func (r *runNowRegistry) Get(context.Context, string) (schedule.Schedule, error) {
	return r.schedule, nil
}
func (r *runNowRegistry) Create(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	r.created = sc
	return sc, nil
}
func (r *runNowRegistry) Update(_ context.Context, sc schedule.Schedule, _ uint64) (schedule.Schedule, error) {
	r.updated = sc
	return sc, nil
}
func (r *runNowRegistry) Delete(context.Context, string) error { return nil }
func (r *runNowRegistry) Due(context.Context, time.Time, int) ([]schedule.Schedule, error) {
	return nil, nil
}
func (r *runNowRegistry) Claim(context.Context, schedule.Occurrence) (bool, error) { return false, nil }
func (r *runNowRegistry) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return nil, nil
}
func (r *runNowRegistry) RecordRun(ctx context.Context, id string, at time.Time) error {
	r.recordedID, r.recordedAt, r.recordCtxErr = id, at, ctx.Err()
	return nil
}

// TestRunWorkerNoOpWithoutScheduling ensures a disabled schedule capability
// returns at once rather than entering a scan loop.
func TestRunWorkerNoOpWithoutWorker(t *testing.T) {
	firing := NewFiring(nil, nil)
	done := make(chan struct{})
	go func() {
		firing.RunWorker(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunWorker blocked without a worker store")
	}
}

// pagedRegistry seeks the way the store does: newest created first, id last so the
// order is total, and a zero anchor is the first page rather than a position before
// every row.
type pagedRegistry struct {
	*runNowRegistry
	rows []schedule.Schedule

	afterID string
	limit   int
}

func (r *pagedRegistry) ListPage(_ context.Context, afterCreatedAt int64, afterID string, limit int) ([]schedule.Schedule, error) {
	r.afterID, r.limit = afterID, limit
	var out []schedule.Schedule
	for _, row := range r.rows {
		if afterCreatedAt != 0 || afterID != "" {
			position := row.CreatedAt.UnixNano()
			if position > afterCreatedAt || (position == afterCreatedAt && row.ID <= afterID) {
				continue
			}
		}
		if limit > 0 && len(out) == limit {
			break
		}
		out = append(out, row)
	}
	return out, nil
}

func scheduleRows(ids ...string) []schedule.Schedule {
	out := make([]schedule.Schedule, 0, len(ids))
	for i, id := range ids {
		out = append(out, schedule.Schedule{ID: id, CreatedAt: time.Unix(0, int64(len(ids)-i)).UTC()})
	}
	return out
}

// TestListPagePagesNewestFirstAndRefusesAForeignCursor covers the schedules query
// properties: the order is fixed (newest created first, id breaking ties), the
// next page seeks strictly past the previous one, and a cursor from another query is
// refused rather than quietly restarting — a schedule shown twice reads as a second
// schedule that fires on the same cron.
func TestListPagePagesNewestFirstAndRefusesAForeignCursor(t *testing.T) {
	registry := &pagedRegistry{runNowRegistry: &runNowRegistry{}, rows: scheduleRows("sch_1", "sch_2", "sch_3")}
	c := New(Dependencies{Store: registry})
	ctx := t.Context()

	first, err := c.ListPage(ctx, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if registry.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", registry.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID != "sch_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two schedules and a cursor", first.Rows)
	}

	second, err := c.ListPage(ctx, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if registry.afterID != "sch_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", registry.afterID)
	}
	if len(second.Rows) != 1 || second.Rows[0].ID != "sch_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}

	foreign := keyset.Encode("sessions", nil, []string{"0", "sch_1"})
	if _, err := c.ListPage(ctx, foreign, 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("cursor from another query err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListPage(ctx, first.NextCursor+"x", 2); !errors.Is(err, keyset.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}
}
