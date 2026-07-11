package schedules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

type workerStore struct {
	due       []schedule.Schedule
	dueErr    error
	markCalls []markCall
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
func (s *workerStore) Update(context.Context, schedule.Schedule) (schedule.Schedule, error) {
	return schedule.Schedule{}, nil
}
func (s *workerStore) Delete(context.Context, string) error { return nil }
func (s *workerStore) Due(_ context.Context, now time.Time) ([]schedule.Schedule, error) {
	return s.due, s.dueErr
}
func (s *workerStore) MarkFired(_ context.Context, id string, ranAt, prevNextRunAt, nextRunAt time.Time) error {
	s.markCalls = append(s.markCalls, markCall{id: id, ranAt: ranAt, prevNextRunAt: prevNextRunAt, nextRunAt: nextRunAt})
	return nil
}
func (s *workerStore) RecordRun(context.Context, string, time.Time) error { return nil }

type workerRunner struct {
	err   error
	fired []schedule.Schedule
}

func (r *workerRunner) StartScheduledRun(_ context.Context, sc schedule.Schedule) (string, error) {
	r.fired = append(r.fired, sc)
	if r.err != nil {
		return "", r.err
	}
	return "ses_1", nil
}

func TestWorkerFireDueMarksFiredAfterRunFailure(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	prev := now.Add(-time.Minute)
	store := &workerStore{due: []schedule.Schedule{{ID: "sch_1", Cron: "* * * * *", NextRunAt: prev}}}
	runner := &workerRunner{err: errors.New("boom")}

	NewWorker(store, runner).fireDue(context.Background(), now)

	if len(runner.fired) != 1 || runner.fired[0].ID != "sch_1" {
		t.Fatalf("fired = %+v, want sch_1", runner.fired)
	}
	if len(store.markCalls) != 1 {
		t.Fatalf("mark calls = %d, want 1", len(store.markCalls))
	}
	call := store.markCalls[0]
	if call.id != "sch_1" || !call.ranAt.Equal(now) || !call.prevNextRunAt.Equal(prev) {
		t.Fatalf("mark call = %+v", call)
	}
	if !call.nextRunAt.After(now) {
		t.Fatalf("next run = %v, want after %v", call.nextRunAt, now)
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
