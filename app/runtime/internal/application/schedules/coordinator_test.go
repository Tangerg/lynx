package schedules

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/application/pagination"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// TestNilRegistryDisablesCRUD: a coordinator built without a store reports
// every CRUD op as unavailable (the no-scheduling build), rather than panicking.
func TestNilRegistryDisablesCRUD(t *testing.T) {
	c := New(Dependencies{})
	ctx := context.Background()

	if c.Available() {
		t.Fatal("Available = true, want false")
	}
	if _, err := c.List(ctx); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("List err = %v, want ErrUnavailable", err)
	}
	if _, err := c.ListPage(ctx, "", 1); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListPage err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Create(ctx, CreateCommand{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Create err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Update(ctx, UpdateCommand{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Update err = %v, want ErrUnavailable", err)
	}
	if err := c.Delete(ctx, "sch_1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Delete err = %v, want ErrUnavailable", err)
	}
	firing := NewFiring(nil, nil, nil)
	if firing.Available() {
		t.Fatal("firing Available = true, want false")
	}
	if _, err := firing.RunNow(ctx, "sch_1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("RunNow err = %v, want ErrUnavailable", err)
	}
}

func TestRunNowRecordsAcceptedRunAfterRequestCancellation(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	store := &runNowStore{schedule: schedule.Schedule{ID: "sch_1", Instructions: "review"}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: true}
	var notices []invalidation.Notice
	firing := NewFiring(store, &runner, func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})
	firing.now = func() time.Time { return now }

	if _, err := firing.RunNow(ctx, "sch_1"); err != nil {
		t.Fatalf("RunNow: %v", err)
	}
	if store.recordedID != "sch_1" || !store.recordedAt.Equal(now) {
		t.Fatalf("recorded = (%q, %v), want (sch_1, %v)", store.recordedID, store.recordedAt, now)
	}
	if store.recordCtxErr != nil {
		t.Fatalf("record context error = %v, want live post-accept context", store.recordCtxErr)
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.Schedules ||
		!slices.Equal(notices[0].ScheduleIDs, []string{"sch_1"}) {
		t.Fatalf("post-record invalidations = %+v, want schedules/sch_1", notices)
	}
}

func TestRunNowDoesNotRecordCancellationAbortedRun(t *testing.T) {
	store := &runNowStore{schedule: schedule.Schedule{ID: "sch_1", Instructions: "review"}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: false}
	firing := NewFiring(store, &runner, nil)

	if _, err := firing.RunNow(ctx, "sch_1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunNow error = %v, want context.Canceled", err)
	}
	if store.recordedID != "" {
		t.Fatalf("recorded id = %q, want none", store.recordedID)
	}
}

func TestRunNowRecordFailureDoesNotPublishSchedule(t *testing.T) {
	store := &runNowStore{
		schedule:  schedule.Schedule{ID: "sch_1", Instructions: "review"},
		recordErr: errors.New("record failed"),
	}
	runner := &cancelingScheduledRunStarter{cancel: func() {}, succeed: true}
	var notices []invalidation.Notice
	firing := NewFiring(store, runner, func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})

	if _, err := firing.RunNow(t.Context(), "sch_1"); err == nil {
		t.Fatal("RunNow error = nil, want record failure")
	}
	if len(notices) != 0 {
		t.Fatalf("failed record invalidations = %+v, want none", notices)
	}
}

type cwdResolverFunc func(string) (string, error)

func (c cwdResolverFunc) ResolveExistingDir(path string) (string, error) {
	return c(path)
}

func TestCreateOwnsScheduleAdmission(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	store := &runNowStore{}
	c := New(Dependencies{
		Store: store,
		Paths: cwdResolverFunc(func(path string) (string, error) {
			if path != "workspace" {
				t.Fatalf("ResolveExistingDir(%q), want workspace", path)
			}
			return "/canonical/workspace", nil
		}),
	})
	c.now = func() time.Time { return now }

	created, err := c.Create(t.Context(), CreateCommand{
		Instructions: "review",
		CWD:          "workspace",
		Cron:         "0 13 * * *",
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.CWD != "/canonical/workspace" || !created.Enabled {
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
	store := &runNowStore{schedule: schedule.Schedule{
		ID:           "sch_1",
		Revision:     3,
		Instructions: "before",
		CWD:          "/before",
		Cron:         "0 9 * * *",
		Enabled:      true,
		LastRunAt:    lastRun,
		CreatedAt:    createdAt,
	}}
	c := New(Dependencies{
		Store: store,
		Paths: cwdResolverFunc(func(string) (string, error) {
			return "/canonical/after", nil
		}),
	})
	cwd, instructions, enabled := "after", "after", false

	updated, err := c.Update(t.Context(), UpdateCommand{
		ID: "sch_1", ExpectedRevision: 3,
		Patch: schedule.Patch{
			Instructions: &instructions,
			CWD:          &cwd,
			Enabled:      &enabled,
		},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ID != "sch_1" || updated.Instructions != "after" || updated.CWD != "/canonical/after" {
		t.Fatalf("updated = %+v", updated)
	}
	if !updated.LastRunAt.Equal(lastRun) || !updated.CreatedAt.Equal(createdAt) || !updated.NextRunAt.IsZero() {
		t.Fatalf("updated durable state = %+v", updated)
	}
}

func TestUpdateRequiresAnExplicitRevision(t *testing.T) {
	c := New(Dependencies{Store: &runNowStore{schedule: schedule.Schedule{ID: "sch_1"}}})
	_, err := c.Update(t.Context(), UpdateCommand{ID: "sch_1"})
	if !errors.Is(err, schedule.ErrRevisionRequired) {
		t.Fatalf("Update error = %v, want ErrRevisionRequired", err)
	}
}

func TestCreateValidatesBeforeResolvingCWD(t *testing.T) {
	resolved := false
	c := New(Dependencies{
		Store: &runNowStore{},
		Paths: cwdResolverFunc(func(string) (string, error) {
			resolved = true
			return "", errors.New("unexpected resolution")
		}),
	})
	_, err := c.Create(t.Context(), CreateCommand{CWD: "missing", Cron: "@daily", Enabled: true})
	if !errors.Is(err, schedule.ErrInstructionsRequired) {
		t.Fatalf("Create error = %v, want ErrInstructionsRequired", err)
	}
	if resolved {
		t.Fatal("cwd was resolved before schedule validation")
	}
}

type runNowStore struct {
	schedule     schedule.Schedule
	created      schedule.Schedule
	updated      schedule.Schedule
	recordedID   string
	recordedAt   time.Time
	recordCtxErr error
	recordErr    error
}

func (r *runNowStore) ListPage(ctx context.Context, _ time.Time, _ string, _ int) ([]schedule.Schedule, error) {
	return r.List(ctx)
}

func (r *runNowStore) List(context.Context) ([]schedule.Schedule, error) { return nil, nil }
func (r *runNowStore) Get(context.Context, string) (schedule.Schedule, error) {
	return r.schedule, nil
}
func (r *runNowStore) Create(_ context.Context, scheduled schedule.Schedule) (schedule.Schedule, error) {
	r.created = scheduled
	return scheduled, nil
}
func (r *runNowStore) Update(_ context.Context, scheduled schedule.Schedule, _ uint64) (schedule.Schedule, error) {
	r.updated = scheduled
	return scheduled, nil
}
func (r *runNowStore) Delete(context.Context, string) (bool, error) { return false, nil }
func (r *runNowStore) Due(context.Context, time.Time, int) ([]schedule.Schedule, error) {
	return nil, nil
}
func (r *runNowStore) Claim(context.Context, schedule.Occurrence) (bool, error) { return false, nil }
func (r *runNowStore) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return nil, nil
}
func (r *runNowStore) RecordRun(ctx context.Context, id string, at time.Time) error {
	r.recordedID, r.recordedAt, r.recordCtxErr = id, at, ctx.Err()
	return r.recordErr
}

// TestRunWorkerNoOpWithoutScheduling ensures a disabled schedule capability
// returns at once rather than entering a scan loop.
func TestRunWorkerNoOpWithoutWorker(t *testing.T) {
	firing := NewFiring(nil, nil, nil)
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

// pagedStore seeks the way the store does: newest created first, id last so the
// order is total, and a zero anchor is the first page rather than a position before
// every row.
type pagedStore struct {
	*runNowStore
	rows []schedule.Schedule

	afterID string
	limit   int
}

func (p *pagedStore) ListPage(_ context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]schedule.Schedule, error) {
	p.afterID, p.limit = afterID, limit
	var out []schedule.Schedule
	for _, row := range p.rows {
		if !afterCreatedAt.IsZero() || afterID != "" {
			if row.CreatedAt.After(afterCreatedAt) || (row.CreatedAt.Equal(afterCreatedAt) && row.ID <= afterID) {
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
	store := &pagedStore{runNowStore: &runNowStore{}, rows: scheduleRows("sch_1", "sch_2", "sch_3")}
	c := New(Dependencies{Store: store})
	ctx := t.Context()

	first, err := c.ListPage(ctx, "", 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if store.limit != 3 {
		t.Fatalf("store asked for %d rows, want the page plus one", store.limit)
	}
	if len(first.Rows) != 2 || first.Rows[0].ID != "sch_1" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two schedules and a cursor", first.Rows)
	}

	second, err := c.ListPage(ctx, first.NextCursor, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if store.afterID != "sch_2" {
		t.Fatalf("second page sought past %q, want the first page's last row", store.afterID)
	}
	if len(second.Rows) != 1 || second.Rows[0].ID != "sch_3" || second.NextCursor != "" {
		t.Fatalf("second page = %+v, want the tail and no cursor", second.Rows)
	}

	foreign := pagination.Encode("sessions", nil, []string{"0", "sch_1"})
	if _, err := c.ListPage(ctx, foreign, 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("cursor from another query err = %v, want ErrInvalidCursor", err)
	}
	if _, err := c.ListPage(ctx, first.NextCursor+"x", 2); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("damaged cursor err = %v, want ErrInvalidCursor", err)
	}
}
