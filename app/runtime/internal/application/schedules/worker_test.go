package schedules

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type workerStore struct {
	due                []schedule.Schedule
	dueErr             error
	claims             []claimRecord
	claimContextErrors []error
	pending            []schedule.Occurrence
	claimed            map[string]bool
}

type claimRecord struct {
	id            string
	ranAt         time.Time
	prevNextRunAt time.Time
	nextRunAt     time.Time
}

func (w *workerStore) List(context.Context) ([]schedule.Schedule, error) { return nil, nil }
func (w *workerStore) Get(context.Context, string) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrNotFound
}
func (w *workerStore) Create(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (w *workerStore) Update(context.Context, schedule.Schedule, uint64) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (w *workerStore) Delete(context.Context, string) (bool, error) { return false, nil }
func (w *workerStore) Due(_ context.Context, _ time.Time, _ int) ([]schedule.Schedule, error) {
	return w.due, w.dueErr
}
func (w *workerStore) Claim(ctx context.Context, occurrence schedule.Occurrence) (bool, error) {
	if w.claimed == nil {
		w.claimed = map[string]bool{}
	}
	if w.claimed[occurrence.ID] {
		return false, nil
	}
	w.claimed[occurrence.ID] = true
	w.claims = append(w.claims, claimRecord{id: occurrence.Schedule.ID, ranAt: occurrence.FiredAt, prevNextRunAt: occurrence.DueAt, nextRunAt: occurrence.NextRunAt})
	w.claimContextErrors = append(w.claimContextErrors, ctx.Err())
	w.pending = append(w.pending, occurrence)
	return true, nil
}
func (w *workerStore) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return w.pending, nil
}
func (w *workerStore) RecordRun(context.Context, string, time.Time) error { return nil }

type recordingScheduledRunStarter struct {
	startErr         error
	startedSchedules []schedule.Schedule
}

func (r *recordingScheduledRunStarter) StartScheduledRun(_ context.Context, occurrence schedule.Occurrence) (StartedRun, error) {
	r.startedSchedules = append(r.startedSchedules, occurrence.Schedule)
	if r.startErr != nil {
		return StartedRun{}, r.startErr
	}
	return StartedRun{SessionID: "ses_1", RunID: "run_1"}, nil
}

func TestWorkerFireDueLeavesFailedOccurrenceDue(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	prev := now.Add(-time.Minute)
	store := &workerStore{due: []schedule.Schedule{{ID: "sch_1", Cron: "* * * * *", NextRunAt: prev}}}
	runner := &recordingScheduledRunStarter{startErr: errors.New("boom")}
	var notices []invalidation.Notice
	w := NewWorker(store, runner, func(notice invalidation.Notice) {
		notices = append(notices, notice)
	})

	// The durable due row is intentionally presented again on the next scan: a
	// rejected run must never be recorded as fired, even after a process restart.
	w.fireDue(context.Background(), now)
	w.fireDue(context.Background(), now)
	if len(runner.startedSchedules) != 2 {
		t.Fatalf("started = %d, want 2", len(runner.startedSchedules))
	}
	if len(store.claims) != 1 {
		t.Fatalf("claims = %d, want one durable occurrence", len(store.claims))
	}
	if len(notices) != 1 || !slices.Equal(notices[0].ScheduleIDs, []string{"sch_1"}) {
		t.Fatalf("claim invalidations = %+v, want one cursor change", notices)
	}
}

func TestWorkerFireDueDisablesCorruptCron(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{{ID: "sch_bad", Cron: "not cron", NextRunAt: now}}}
	runner := &recordingScheduledRunStarter{}

	NewWorker(store, runner, nil).fireDue(context.Background(), now)

	if len(runner.startedSchedules) != 1 {
		t.Fatalf("started = %d, want 1", len(runner.startedSchedules))
	}
	if len(store.claims) != 1 || !store.claims[0].nextRunAt.IsZero() {
		t.Fatalf("claims = %+v, want zero nextRunAt", store.claims)
	}
}

func TestWorkerFireDueStopsOnDueError(t *testing.T) {
	store := &workerStore{dueErr: errors.New("db down")}
	runner := &recordingScheduledRunStarter{}

	NewWorker(store, runner, nil).fireDue(context.Background(), time.Now())

	if len(runner.startedSchedules) != 0 || len(store.claims) != 0 {
		t.Fatalf("started=%d claims=%d, want none", len(runner.startedSchedules), len(store.claims))
	}
}

func TestWorkerFireDueDoesNotConsumeCancellationAbortedFiring(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{
		{ID: "sch_1", Cron: "* * * * *", NextRunAt: now},
		{ID: "sch_2", Cron: "* * * * *", NextRunAt: now},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: false}

	NewWorker(store, &runner, nil).fireDue(ctx, now)

	if len(runner.startedScheduleIDs) != 1 || runner.startedScheduleIDs[0] != "sch_1" {
		t.Fatalf("started = %v, want only sch_1", runner.startedScheduleIDs)
	}
	if len(store.claims) != 1 {
		t.Fatalf("claims = %+v, want only the occurrence dispatched before cancellation", store.claims)
	}
}

func TestWorkerFireDuePersistsAcceptedFiringAfterCancellation(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{
		{ID: "sch_1", Cron: "* * * * *", NextRunAt: now},
		{ID: "sch_2", Cron: "* * * * *", NextRunAt: now},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: true}

	NewWorker(store, &runner, nil).fireDue(ctx, now)

	if len(runner.startedScheduleIDs) != 1 || runner.startedScheduleIDs[0] != "sch_1" {
		t.Fatalf("started = %v, want only sch_1", runner.startedScheduleIDs)
	}
	if len(store.claims) != 1 || store.claims[0].id != "sch_1" {
		t.Fatalf("claims = %+v, want only sch_1", store.claims)
	}
	if len(store.claimContextErrors) != 1 || store.claimContextErrors[0] != nil {
		t.Fatalf("claim context errors = %v, want live context", store.claimContextErrors)
	}
}

func TestWorkerRunScansImmediately(t *testing.T) {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{{ID: "sch_1", Cron: "* * * * *", NextRunAt: now}}}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	runner := cancelingScheduledRunStarter{cancel: cancel, succeed: true}
	worker := NewWorker(store, &runner, nil)
	worker.now = func() time.Time { return now }

	done := make(chan struct{})
	go func() {
		worker.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop after its initial scan")
	}
	if len(runner.startedScheduleIDs) != 1 || runner.startedScheduleIDs[0] != "sch_1" {
		t.Fatalf("initial scan started = %v, want [sch_1]", runner.startedScheduleIDs)
	}
}

type cancelingScheduledRunStarter struct {
	cancel             context.CancelFunc
	succeed            bool
	startedScheduleIDs []string
}

func (c *cancelingScheduledRunStarter) StartScheduledRun(ctx context.Context, occurrence schedule.Occurrence) (StartedRun, error) {
	c.startedScheduleIDs = append(c.startedScheduleIDs, occurrence.Schedule.ID)
	c.cancel()
	if !c.succeed {
		return StartedRun{}, ctx.Err()
	}
	return StartedRun{SessionID: "ses_1", RunID: "run_1"}, nil
}
