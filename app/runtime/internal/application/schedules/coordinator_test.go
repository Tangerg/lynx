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
	c := NewCoordinator(nil, nil)
	ctx := context.Background()

	if _, err := c.List(ctx); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("List err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Get(ctx, "sch_1"); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("Get err = %v, want ErrUnavailable", err)
	}
	if _, err := c.Create(ctx, schedule.Schedule{}); !errors.Is(err, schedule.ErrUnavailable) {
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
	c := NewCoordinator(registry, nil)
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
	c := NewCoordinator(registry, nil)
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingWorkerRunner{cancel: cancel, succeed: false}

	if err := c.RunNow(ctx, "sch_1", &runner); !errors.Is(err, context.Canceled) {
		t.Fatalf("RunNow error = %v, want context.Canceled", err)
	}
	if registry.recordedID != "" {
		t.Fatalf("recorded id = %q, want none", registry.recordedID)
	}
}

type runNowRegistry struct {
	schedule     schedule.Schedule
	recordedID   string
	recordedAt   time.Time
	recordCtxErr error
}

func (r *runNowRegistry) List(context.Context) ([]schedule.Schedule, error) { return nil, nil }
func (r *runNowRegistry) Get(context.Context, string) (schedule.Schedule, error) {
	return r.schedule, nil
}
func (r *runNowRegistry) Create(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (r *runNowRegistry) Update(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
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
	c := NewCoordinator(nil, nil)
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
