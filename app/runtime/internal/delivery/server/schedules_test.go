package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/worktree"
)

type scheduleRuntime struct {
	stubRuntime
	listed       []schedule.Schedule
	byID         map[string]schedule.Schedule
	created      []schedule.Schedule
	updated      []schedule.Schedule
	deleted      []string
	workerRunner schedule.Runner
}

func (r *scheduleRuntime) ListSchedules(context.Context) ([]schedule.Schedule, error) {
	return r.listed, nil
}

func (r *scheduleRuntime) GetSchedule(_ context.Context, id string) (schedule.Schedule, error) {
	sc, ok := r.byID[id]
	if !ok {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	return sc, nil
}

func (r *scheduleRuntime) CreateSchedule(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	r.created = append(r.created, sc)
	if sc.ID == "" {
		sc.ID = "sch_created"
	}
	if sc.CreatedAt.IsZero() {
		sc.CreatedAt = time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	}
	return sc, nil
}

func (r *scheduleRuntime) UpdateSchedule(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	r.updated = append(r.updated, sc)
	return sc, nil
}

func (r *scheduleRuntime) DeleteSchedule(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *scheduleRuntime) RecordScheduleRun(context.Context, string, time.Time) error {
	return nil
}

func (r *scheduleRuntime) RunScheduleWorker(_ context.Context, runner schedule.Runner) {
	r.workerRunner = runner
}

func TestCreateScheduleBuildsEnabledDomainSchedule(t *testing.T) {
	rt := &scheduleRuntime{}
	s := newTestServer(rt)
	cwd := t.TempDir()

	got, err := s.CreateSchedule(context.Background(), protocol.CreateScheduleRequest{
		Title:  "Morning",
		Prompt: "Summarize the repo",
		Cwd:    cwd,
		Cron:   "@daily",
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if len(rt.created) != 1 {
		t.Fatalf("created %d schedule(s), want 1", len(rt.created))
	}
	created := rt.created[0]
	if !created.Enabled || created.Prompt != "Summarize the repo" || created.Cwd != worktree.CanonicalCwd(cwd) || created.Cron != "@daily" {
		t.Fatalf("created = %+v", created)
	}
	if created.NextRunAt.IsZero() {
		t.Fatal("created.NextRunAt is zero, want computed first run")
	}
	if got.ID != "sch_created" || got.NextRunAt == nil {
		t.Fatalf("wire schedule = %+v, want id and nextRunAt", got)
	}
}

func TestCreateScheduleRejectsUnavailableCwd(t *testing.T) {
	rt := &scheduleRuntime{}
	s := newTestServer(rt)

	_, err := s.CreateSchedule(context.Background(), protocol.CreateScheduleRequest{
		Prompt: "Summarize the repo",
		Cwd:    t.TempDir() + "/missing",
		Cron:   "@daily",
	})
	if !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("create schedule cwd err = %v, want ErrCwdUnavailable", err)
	}
	if len(rt.created) != 0 {
		t.Fatalf("created %d schedule(s), want 0", len(rt.created))
	}
}

func TestUpdateSchedulePreservesStoredTimestampsAndCanDisable(t *testing.T) {
	last := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	rt := &scheduleRuntime{byID: map[string]schedule.Schedule{
		"sch_1": {ID: "sch_1", LastRunAt: last, CreatedAt: createdAt, NextRunAt: last.Add(time.Hour)},
	}}
	s := newTestServer(rt)
	cwd := t.TempDir()

	got, err := s.UpdateSchedule(context.Background(), protocol.UpdateScheduleRequest{
		ID:      "sch_1",
		Title:   "Disabled",
		Prompt:  "Stand down",
		Cwd:     cwd,
		Cron:    "@daily",
		Enabled: false,
	})
	if err != nil {
		t.Fatalf("update schedule: %v", err)
	}
	if len(rt.updated) != 1 {
		t.Fatalf("updated %d schedule(s), want 1", len(rt.updated))
	}
	updated := rt.updated[0]
	if !updated.LastRunAt.Equal(last) || !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("updated timestamps = last %v created %v", updated.LastRunAt, updated.CreatedAt)
	}
	if !updated.NextRunAt.IsZero() {
		t.Fatalf("updated.NextRunAt = %v, want zero when disabled", updated.NextRunAt)
	}
	if updated.Cwd != worktree.CanonicalCwd(cwd) {
		t.Fatalf("updated.Cwd = %q, want %q", updated.Cwd, worktree.CanonicalCwd(cwd))
	}
	if got.NextRunAt != nil || got.LastRunAt == nil {
		t.Fatalf("wire schedule = %+v, want omitted nextRunAt and present lastRunAt", got)
	}
}

func TestUpdateScheduleUnknownIDIsInvalidParams(t *testing.T) {
	s := newTestServer(&scheduleRuntime{})

	_, err := s.UpdateSchedule(context.Background(), protocol.UpdateScheduleRequest{
		ID:      "missing",
		Prompt:  "hello",
		Cron:    "@daily",
		Enabled: true,
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("update missing err = %v, want ErrInvalidParams", err)
	}
}

func TestRunSchedulerDelegatesWorkerToRuntime(t *testing.T) {
	rt := &scheduleRuntime{}
	s := newTestServer(rt)

	s.RunScheduler(context.Background())

	if rt.workerRunner == nil {
		t.Fatal("worker runner not passed to runtime")
	}
}
