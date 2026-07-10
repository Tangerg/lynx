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
	if err := c.RecordRun(ctx, "sch_1", time.Now()); !errors.Is(err, schedule.ErrUnavailable) {
		t.Fatalf("RecordRun err = %v, want ErrUnavailable", err)
	}
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
