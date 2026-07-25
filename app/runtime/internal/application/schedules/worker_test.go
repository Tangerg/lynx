package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type workerStore struct {
	due         []schedule.Schedule
	dueErr      error
	markCalls   []markCall
	markCtxErrs []error
	pending     []schedule.Occurrence
	claimed     map[string]bool
}

type markCall struct {
	id            string
	ranAt         time.Time
	prevNextRunAt time.Time
	nextRunAt     time.Time
}

func (s *workerStore) List(context.Context) ([]schedule.Schedule, error) { return nil, nil }
func (s *workerStore) Get(context.Context, string) (schedule.Schedule, error) {
	return schedule.Schedule{}, schedule.ErrNotFound
}
func (s *workerStore) Create(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (s *workerStore) Update(context.Context, schedule.Schedule, uint64) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (s *workerStore) Delete(context.Context, string) error { return nil }
func (s *workerStore) Due(_ context.Context, _ time.Time, _ int) ([]schedule.Schedule, error) {
	return s.due, s.dueErr
}
func (s *workerStore) Claim(ctx context.Context, occurrence schedule.Occurrence) (bool, error) {
	if s.claimed == nil {
		s.claimed = map[string]bool{}
	}
	if s.claimed[occurrence.ID] {
		return false, nil
	}
	s.claimed[occurrence.ID] = true
	s.markCalls = append(s.markCalls, markCall{id: occurrence.Schedule.ID, ranAt: occurrence.FiredAt, prevNextRunAt: occurrence.DueAt, nextRunAt: occurrence.NextRunAt})
	s.markCtxErrs = append(s.markCtxErrs, ctx.Err())
	s.pending = append(s.pending, occurrence)
	return true, nil
}
func (s *workerStore) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return s.pending, nil
}
func (s *workerStore) RecordRun(context.Context, string, time.Time) error { return nil }

type workerRunner struct {
	err   error
	fired []schedule.Schedule
}

func (r *workerRunner) StartScheduledRun(_ context.Context, occurrence schedule.Occurrence) (RunHandle, error) {
	r.fired = append(r.fired, occurrence.Schedule)
	if r.err != nil {
		return RunHandle{}, r.err
	}
	return RunHandle{SessionID: "ses_1", RunID: "run_1"}, nil
}

func TestWorkerFireDueLeavesFailedOccurrenceDue(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	prev := now.Add(-time.Minute)
	store := &workerStore{due: []schedule.Schedule{{ID: "sch_1", Cron: "* * * * *", NextRunAt: prev}}}
	runner := &workerRunner{err: errors.New("boom")}
	w := NewWorker(store, runner)

	// The durable due row is intentionally presented again on the next scan: a
	// rejected run must never be recorded as fired, even after a process restart.
	w.fireDue(context.Background(), now)
	w.fireDue(context.Background(), now)
	if len(runner.fired) != 2 {
		t.Fatalf("fired = %d, want 2", len(runner.fired))
	}
	if len(store.markCalls) != 1 {
		t.Fatalf("claims = %d, want one durable occurrence", len(store.markCalls))
	}
}

func TestWorkerFireDueDisablesCorruptCron(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{{ID: "sch_bad", Cron: "not cron", NextRunAt: now}}}
	runner := &workerRunner{}

	NewWorker(store, runner).fireDue(context.Background(), now)

	if len(runner.fired) != 1 {
		t.Fatalf("fired = %d, want 1", len(runner.fired))
	}
	if len(store.markCalls) != 1 || !store.markCalls[0].nextRunAt.IsZero() {
		t.Fatalf("mark calls = %+v, want zero nextRunAt", store.markCalls)
	}
}

func TestWorkerFireDueStopsOnDueError(t *testing.T) {
	store := &workerStore{dueErr: errors.New("db down")}
	runner := &workerRunner{}

	NewWorker(store, runner).fireDue(context.Background(), time.Now())

	if len(runner.fired) != 0 || len(store.markCalls) != 0 {
		t.Fatalf("fired=%d marks=%d, want none", len(runner.fired), len(store.markCalls))
	}
}

func TestWorkerFireDueDoesNotConsumeCancellationAbortedFiring(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{
		{ID: "sch_1", Cron: "* * * * *", NextRunAt: now},
		{ID: "sch_2", Cron: "* * * * *", NextRunAt: now},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingWorkerRunner{cancel: cancel, succeed: false}

	NewWorker(store, &runner).fireDue(ctx, now)

	if len(runner.fired) != 1 || runner.fired[0] != "sch_1" {
		t.Fatalf("fired = %v, want only sch_1", runner.fired)
	}
	if len(store.markCalls) != 1 {
		t.Fatalf("claims = %+v, want only the occurrence dispatched before cancellation", store.markCalls)
	}
}

func TestWorkerFireDuePersistsAcceptedFiringAfterCancellation(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{
		{ID: "sch_1", Cron: "* * * * *", NextRunAt: now},
		{ID: "sch_2", Cron: "* * * * *", NextRunAt: now},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	runner := cancelingWorkerRunner{cancel: cancel, succeed: true}

	NewWorker(store, &runner).fireDue(ctx, now)

	if len(runner.fired) != 1 || runner.fired[0] != "sch_1" {
		t.Fatalf("fired = %v, want only sch_1", runner.fired)
	}
	if len(store.markCalls) != 1 || store.markCalls[0].id != "sch_1" {
		t.Fatalf("claims = %+v, want only sch_1", store.markCalls)
	}
	if len(store.markCtxErrs) != 1 || store.markCtxErrs[0] != nil {
		t.Fatalf("claim context errors = %v, want live context", store.markCtxErrs)
	}
}

func TestWorkerRunScansImmediately(t *testing.T) {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	store := &workerStore{due: []schedule.Schedule{{ID: "sch_1", Cron: "* * * * *", NextRunAt: now}}}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	runner := cancelingWorkerRunner{cancel: cancel, succeed: true}
	worker := NewWorker(store, &runner)
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
	if len(runner.fired) != 1 || runner.fired[0] != "sch_1" {
		t.Fatalf("initial scan fired = %v, want [sch_1]", runner.fired)
	}
}

type cancelingWorkerRunner struct {
	cancel  context.CancelFunc
	succeed bool
	fired   []string
}

func (r *cancelingWorkerRunner) StartScheduledRun(ctx context.Context, occurrence schedule.Occurrence) (RunHandle, error) {
	r.fired = append(r.fired, occurrence.Schedule.ID)
	r.cancel()
	if !r.succeed {
		return RunHandle{}, ctx.Err()
	}
	return RunHandle{SessionID: "ses_1", RunID: "run_1"}, nil
}
