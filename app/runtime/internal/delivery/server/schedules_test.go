package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	"github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// fakeScheduleRegistry is the combined test store that records the
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

func (r *fakeScheduleRegistry) ListPage(ctx context.Context, _ int64, _ string, _ int) ([]schedule.Schedule, error) {
	return r.List(ctx)
}

func (r *fakeScheduleRegistry) List(context.Context) ([]schedule.Schedule, error) {
	return r.listed, r.listErr
}

func (r *fakeScheduleRegistry) Get(_ context.Context, id string) (schedule.Schedule, error) {
	scheduled, found := r.byID[id]
	if !found {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	return scheduled, nil
}

func (r *fakeScheduleRegistry) Create(_ context.Context, scheduled schedule.Schedule) (schedule.Schedule, error) {
	r.created = append(r.created, scheduled)
	if scheduled.ID == "" {
		scheduled.ID = "sch_created"
	}
	if scheduled.CreatedAt.IsZero() {
		scheduled.CreatedAt = time.Date(2026, 7, 5, 9, 0, 0, 0, time.UTC)
	}
	return scheduled, nil
}

func (r *fakeScheduleRegistry) Update(_ context.Context, scheduled schedule.Schedule, _ uint64) (schedule.Schedule, error) {
	r.updated = append(r.updated, scheduled)
	return scheduled, nil
}

func (r *fakeScheduleRegistry) Delete(_ context.Context, id string) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *fakeScheduleRegistry) RecordRun(context.Context, string, time.Time) error { return nil }

func (r *fakeScheduleRegistry) Due(context.Context, time.Time, int) ([]schedule.Schedule, error) {
	return nil, nil
}

func (r *fakeScheduleRegistry) Claim(context.Context, schedule.Occurrence) (bool, error) {
	return false, nil
}
func (r *fakeScheduleRegistry) Pending(context.Context, int) ([]schedule.Occurrence, error) {
	return nil, nil
}

// serverWithSchedules builds a test Server whose schedules coordinator is backed
// by reg (used as both the CRUD registry and the worker store).
func serverWithSchedules(reg *fakeScheduleRegistry) *Server {
	s := newTestServer(&stubRuntime{})
	s.schedules = schedules.New(schedules.Dependencies{
		Store: reg,
		Paths: workspacepath.Resolver{},
	})
	s.scheduleFiring = schedules.NewFiring(reg, schedules.NewRunLauncher(s.runs, s.serverInfo.DefaultWorkspace.Path, nil))
	s.features.schedules = true
	return s
}

func TestCreateScheduleBuildsEnabledDomainSchedule(t *testing.T) {
	reg := &fakeScheduleRegistry{}
	s := serverWithSchedules(reg)
	cwd := t.TempDir()

	got, err := s.CreateSchedule(context.Background(), protocol.CreateScheduleRequest{
		Title: "Morning", Instructions: "Summarize the repo",
		Workspace: &protocol.WorkspaceRef{Path: cwd}, Cron: "@daily",
	})
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}
	if len(reg.created) != 1 {
		t.Fatalf("created %d schedule(s), want 1", len(reg.created))
	}
	created := reg.created[0]
	if !created.Enabled || created.Instructions != "Summarize the repo" || created.CWD != canonicalWorkspacePath(t, cwd) || created.Cron != "@daily" {
		t.Fatalf("created = %+v", created)
	}
	if created.NextRunAt.IsZero() {
		t.Fatal("created.NextRunAt is zero, want computed first run")
	}
	if got.ID != "sch_created" || got.NextRunAt == nil {
		t.Fatalf("wire schedule = %+v, want id and nextRunAt", got)
	}
}

func TestCreateScheduleRejectsUnavailableCWD(t *testing.T) {
	reg := &fakeScheduleRegistry{}
	s := serverWithSchedules(reg)

	_, err := s.CreateSchedule(context.Background(), protocol.CreateScheduleRequest{
		Instructions: "Summarize the repo",
		Workspace:    &protocol.WorkspaceRef{Path: t.TempDir() + "/missing"}, Cron: "@daily",
	})
	if !errors.Is(err, protocol.ErrWorkspaceUnavailable) {
		t.Fatalf("create schedule workspace err = %v, want ErrWorkspaceUnavailable", err)
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
		ID:               "sch_1",
		ExpectedRevision: 1,
		Title:            valuePtr("Disabled"),
		Instructions:     valuePtr("Stand down"),
		Workspace:        &protocol.WorkspaceRef{Path: cwd},
		Cron:             valuePtr("@daily"),
		Enabled:          valuePtr(false),
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
	if updated.CWD != canonicalWorkspacePath(t, cwd) {
		t.Fatalf("updated.CWD = %q, want %q", updated.CWD, canonicalWorkspacePath(t, cwd))
	}
	if got.NextRunAt != nil || got.LastRunAt == nil {
		t.Fatalf("wire schedule = %+v, want omitted nextRunAt and present lastRunAt", got)
	}
}

func TestUpdateScheduleUnknownIDIsInvalidParams(t *testing.T) {
	s := serverWithSchedules(&fakeScheduleRegistry{})

	_, err := s.UpdateSchedule(context.Background(), protocol.UpdateScheduleRequest{
		ID:               "missing",
		ExpectedRevision: 1,
		Instructions:     valuePtr("hello"),
		Cron:             valuePtr("@daily"),
		Enabled:          valuePtr(true),
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("update missing err = %v, want ErrInvalidParams", err)
	}
}

func valuePtr[T any](value T) *T { return &value }

func TestScheduleUnavailableIsCapabilityNotNegotiated(t *testing.T) {
	reg := &fakeScheduleRegistry{listErr: schedules.ErrUnavailable}
	s := serverWithSchedules(reg)

	_, err := s.ListSchedules(context.Background(), protocol.PageQuery{})
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("list unavailable err = %v, want capability_not_negotiated", err)
	}
}
