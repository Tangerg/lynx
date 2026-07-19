package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

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
	if _, err := c.Get(ctx, "sch_1"); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("Get err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Create(ctx, CreateCommand{}); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("Create err = %v, want ErrUnavailable", err)
	}
	if err := c.Delete(ctx, "sch_1"); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("Delete err = %v, want ErrUnavailable", err)
	}
	if err := c.RunNow(ctx, "sch_1", nil); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("RunNow err = %v, want ErrUnavailable", err)
	}
}

func TestRunNowRecordsAcceptedRunAfterRequestCancellation(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	registry := &runNowRegistry{schedule: schedule.Schedule{ID: "sch_1", Prompt: "review"}}
	c := New(Dependencies{Registry: registry})
	c.now = func() time.Time { return now }
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingWorkerRunner{cancel: cancel, succeed: true}

	if err := c.RunNow(ctx, "sch_1", &runner); err != nil {
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
	c := New(Dependencies{Registry: registry})
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingWorkerRunner{cancel: cancel, succeed: false}

	if err := c.RunNow(ctx, "sch_1", &runner); !errors.Is(err, context.Canceled) {
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
		Registry: registry,
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

func TestUpdateOwnsPatchAndPreservesDurableState(t *testing.T) {
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
		Registry: registry,
		Paths: cwdResolverFunc(func(string) (string, error) {
			return "/canonical/after", nil
		}),
	})
	cwd, prompt, enabled := "after", "after", false

	updated, err := c.Update(t.Context(), UpdateCommand{
		ID: "sch_1",
		Patch: schedule.Patch{
			Prompt:  &prompt,
			Cwd:     &cwd,
			Enabled: &enabled,
		},
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

func TestCreateValidatesBeforeResolvingCwd(t *testing.T) {
	resolved := false
	c := New(Dependencies{
		Registry: &runNowRegistry{},
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

func (r *runNowRegistry) List(context.Context) ([]schedule.Schedule, error) { return nil, nil }
func (r *runNowRegistry) Get(context.Context, string) (schedule.Schedule, error) {
	return r.schedule, nil
}
func (r *runNowRegistry) Create(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	r.created = sc
	return sc, nil
}
func (r *runNowRegistry) Update(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	r.updated = sc
	return sc, nil
}
func (r *runNowRegistry) Delete(context.Context, string) error { return nil }
func (r *runNowRegistry) Due(context.Context, time.Time) ([]schedule.Schedule, error) {
	return nil, nil
}
func (r *runNowRegistry) MarkFired(context.Context, string, time.Time, time.Time, time.Time) error {
	return nil
}
func (r *runNowRegistry) RecordRun(ctx context.Context, id string, at time.Time) error {
	r.recordedID, r.recordedAt, r.recordCtxErr = id, at, ctx.Err()
	return nil
}

// TestRunWorkerNoOpWithoutWorker: no worker store → RunWorker returns at once
// instead of entering the scan loop, so a build without scheduling doesn't hang.
func TestRunWorkerNoOpWithoutWorker(t *testing.T) {
	c := New(Dependencies{})
	done := make(chan struct{})
	go func() {
		c.RunWorker(context.Background(), nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunWorker blocked without a worker store")
	}
}
