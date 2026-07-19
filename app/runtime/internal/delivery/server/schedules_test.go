package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// fakeScheduleRegistry is a schedule.Registry (+ WorkerStore) that records the
// CRUD the schedules coordinator drives, so delivery tests assert the wire→domain
// mapping without a real store.
type fakeScheduleRegistry struct {
	listed  []schedule.Schedule
	listErr error
	byID    map[string]schedule.Schedule
	created []schedule.Schedule
	updated []schedule.Schedule
	deleted []string
}

func (r *fakeScheduleRegistry) List(context.Context) ([]schedule.Schedule, error) {
	return r.listed, r.listErr
}

func (r *fakeScheduleRegistry) Get(_ context.Context, id string) (schedule.Schedule, error) {
	sc, ok := r.byID[id]
	if !ok {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	return sc, nil
}

func (r *fakeScheduleRegistry) Create(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	r.created = append(r.created, sc)
	if sc.ID == "" {
		sc.ID = "sch_created"
	}
	if sc.CreatedAt.IsZero() {
		sc.CreatedAt = time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	}
	return sc, nil
}

func (r *fakeScheduleRegistry) Update(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	r.updated = append(r.updated, sc)
	return sc, nil
}

func (r *fakeScheduleRegistry) Delete(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *fakeScheduleRegistry) RecordRun(context.Context, string, time.Time) error { return nil }

func (r *fakeScheduleRegistry) Due(context.Context, time.Time) ([]schedule.Schedule, error) {
	return nil, nil
}

func (r *fakeScheduleRegistry) MarkFired(context.Context, string, time.Time, time.Time, time.Time) error {
	return nil
}

// serverWithSchedules builds a test Server whose schedules coordinator is backed
// by reg (used as both the CRUD registry and the worker store).
func serverWithSchedules(reg schedule.Registry) *Server {
	s := newTestServer(&stubRuntime{})
	s.schedules = schedules.New(schedules.Dependencies{
		Registry: reg,
		Worker:   reg,
		Paths:    workspacepath.Resolver{},
	})
	return s
}

func TestCreateScheduleBuildsEnabledDomainSchedule(t *testing.T) {
	reg := &fakeScheduleRegistry{}
	s := serverWithSchedules(reg)
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
	if len(reg.created) != 1 {
		t.Fatalf("created %d schedule(s), want 1", len(reg.created))
	}
	created := reg.created[0]
	if !created.Enabled || created.Prompt != "Summarize the repo" || created.Cwd != workspacepath.Canonical(cwd) || created.Cron != "@daily" {
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
	reg := &fakeScheduleRegistry{}
	s := serverWithSchedules(reg)

	_, err := s.CreateSchedule(context.Background(), protocol.CreateScheduleRequest{
		Prompt: "Summarize the repo",
		Cwd:    t.TempDir() + "/missing",
		Cron:   "@daily",
	})
	if !errors.Is(err, protocol.ErrCwdUnavailable) {
		t.Fatalf("create schedule cwd err = %v, want ErrCwdUnavailable", err)
	}
	if len(reg.created) != 0 {
		t.Fatalf("created %d schedule(s), want 0", len(reg.created))
	}
}

func TestUpdateSchedulePreservesStoredTimestampsAndCanDisable(t *testing.T) {
	last := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	createdAt := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	reg := &fakeScheduleRegistry{byID: map[string]schedule.Schedule{
		"sch_1": {ID: "sch_1", LastRunAt: last, CreatedAt: createdAt, NextRunAt: last.Add(time.Hour)},
	}}
	s := serverWithSchedules(reg)
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
	if len(reg.updated) != 1 {
		t.Fatalf("updated %d schedule(s), want 1", len(reg.updated))
	}
	updated := reg.updated[0]
	if !updated.LastRunAt.Equal(last) || !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("updated timestamps = last %v created %v", updated.LastRunAt, updated.CreatedAt)
	}
	if !updated.NextRunAt.IsZero() {
		t.Fatalf("updated.NextRunAt = %v, want zero when disabled", updated.NextRunAt)
	}
	if updated.Cwd != workspacepath.Canonical(cwd) {
		t.Fatalf("updated.Cwd = %q, want %q", updated.Cwd, workspacepath.Canonical(cwd))
	}
	if got.NextRunAt != nil || got.LastRunAt == nil {
		t.Fatalf("wire schedule = %+v, want omitted nextRunAt and present lastRunAt", got)
	}
}

func TestUpdateScheduleUnknownIDIsInvalidParams(t *testing.T) {
	s := serverWithSchedules(&fakeScheduleRegistry{})

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

func TestScheduleUnavailableIsCapabilityNotNegotiated(t *testing.T) {
	reg := &fakeScheduleRegistry{listErr: schedule.ErrUnavailable}
	s := serverWithSchedules(reg)

	_, err := s.ListSchedules(context.Background())
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("list unavailable err = %v, want capability_not_negotiated", err)
	}
}

// TestRunSchedulerNoOpWhenDisabled: the default (disabled) coordinator has no
// worker store, so RunScheduler returns immediately rather than blocking on a
// scan loop — the delivery wiring's no-scheduling path.
func TestRunSchedulerNoOpWhenDisabled(t *testing.T) {
	s := newTestServer(&stubRuntime{}) // default disabled schedules coordinator
	done := make(chan struct{})
	go func() {
		s.RunScheduler(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunScheduler blocked with no worker store")
	}
}
