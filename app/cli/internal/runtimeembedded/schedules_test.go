package runtimeembedded

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/schedule"
)

type scheduleBindingStub struct {
	t            *testing.T
	now          time.Time
	actions      []string
	keys         map[string]struct{}
	created      protocol.CreateScheduleRequest
	updated      protocol.UpdateScheduleRequest
	createResult *protocol.Schedule
	updateResult *protocol.Schedule
}

func (s *scheduleBindingStub) ListSchedules(_ context.Context, query protocol.PageQuery, options embedded.CallOptions) (*protocol.Page[protocol.Schedule], error) {
	s.assertMeta(options.RequestMeta)
	if query.Limit != schedulePageLimit {
		s.t.Fatalf("schedule page limit = %d", query.Limit)
	}
	first := wireSchedule(s.now, "sch_1")
	if query.Cursor == "" {
		return protocol.NewPageWithCursor([]protocol.Schedule{first}, "next"), nil
	}
	second := wireSchedule(s.now.Add(time.Minute), "sch_2")
	return protocol.NewPage([]protocol.Schedule{second}), nil
}

func (s *scheduleBindingStub) CreateSchedule(_ context.Context, request protocol.CreateScheduleRequest, options embedded.CommandOptions) (*protocol.Schedule, error) {
	s.assertCommand("create", options)
	s.created = request
	if s.createResult != nil {
		return s.createResult, nil
	}
	created := wireSchedule(s.now, "sch_created")
	created.Title, created.Instructions, created.Cron = request.Title, request.Instructions, request.Cron
	created.Provider, created.Model = request.Provider, request.Model
	created.Workspace = request.Workspace
	return &created, nil
}

func (s *scheduleBindingStub) UpdateSchedule(_ context.Context, request protocol.UpdateScheduleRequest, options embedded.CommandOptions) (*protocol.Schedule, error) {
	s.assertCommand("update", options)
	s.updated = request
	if s.updateResult != nil {
		return s.updateResult, nil
	}
	updated := wireSchedule(s.now, request.ID)
	updated.Revision = request.ExpectedRevision + 1
	if request.Title != nil {
		updated.Title = *request.Title
	}
	if request.Instructions != nil {
		updated.Instructions = *request.Instructions
	}
	if request.Workspace != nil {
		updated.Workspace = request.Workspace
	} else if request.WorkspaceMode == protocol.ScheduleWorkspaceDefault {
		updated.Workspace = nil
	}
	if request.Provider != nil {
		updated.Provider = *request.Provider
		updated.Model = *request.Model
	}
	if request.Cron != nil {
		updated.Cron = *request.Cron
	}
	if request.Enabled != nil {
		updated.Enabled = *request.Enabled
		if !updated.Enabled {
			updated.NextRunAt = nil
		}
	}
	return &updated, nil
}

func (s *scheduleBindingStub) DeleteSchedule(_ context.Context, request protocol.DeleteScheduleRequest, options embedded.CommandOptions) error {
	s.assertCommand("delete:"+request.ID, options)
	return nil
}

func (s *scheduleBindingStub) RunScheduleNow(_ context.Context, request protocol.RunScheduleNowRequest, options embedded.CommandOptions) (*protocol.RunScheduleNowResponse, error) {
	s.assertCommand("run:"+request.ID, options)
	return &protocol.RunScheduleNowResponse{SessionID: "ses_headless", RunID: "run_headless"}, nil
}

func (s *scheduleBindingStub) assertMeta(meta protocol.RequestMeta) {
	s.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		s.t.Fatalf("schedule request meta = %+v", meta)
	}
}

func (s *scheduleBindingStub) assertCommand(action string, options embedded.CommandOptions) {
	s.t.Helper()
	s.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		s.t.Fatal("schedule command has no idempotency key")
	}
	if _, duplicate := s.keys[options.IdempotencyKey]; duplicate {
		s.t.Fatalf("schedule command reused idempotency key %q", options.IdempotencyKey)
	}
	s.keys[options.IdempotencyKey] = struct{}{}
	s.actions = append(s.actions, action)
}

func wireSchedule(now time.Time, id string) protocol.Schedule {
	next := now.Add(time.Hour)
	return protocol.Schedule{
		ID: id, Title: "Review", Instructions: "review the repository", Cron: "0 * * * *",
		Enabled: true, NextRunAt: &next, CreatedAt: now, Revision: 1,
	}
}

func TestScheduleAdapterConsumesEveryOperationAndPaginates(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	stub := &scheduleBindingStub{t: t, now: now, keys: make(map[string]struct{})}
	runtime := &Runtime{
		schedules: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}

	listed, err := runtime.Schedules(t.Context())
	if err != nil || len(listed) != 2 || listed[0].ID != "sch_1" || listed[1].ID != "sch_2" {
		t.Fatalf("Schedules = (%+v, %v)", listed, err)
	}
	candidate := schedule.Candidate{
		Title: "Daily review", Instructions: "review everything", Workspace: "/workspace",
		Provider: "deepseek", Model: "deepseek-v4-flash", Cron: "0 9 * * *",
	}
	created, err := runtime.Create(t.Context(), candidate)
	if err != nil || created.ID != "sch_created" {
		t.Fatalf("Create = (%+v, %v)", created, err)
	}
	if stub.created.Workspace == nil || stub.created.Workspace.Path != "/workspace" || stub.created.Model != candidate.Model {
		t.Fatalf("create request = %+v", stub.created)
	}
	title := "Updated"
	updated, err := runtime.Update(t.Context(), schedule.Patch{ID: created.ID, ExpectedRevision: created.Revision, Title: &title})
	if err != nil || updated.Title != title || updated.Revision != created.Revision+1 {
		t.Fatalf("Update = (%+v, %v)", updated, err)
	}
	if stub.updated.ExpectedRevision != created.Revision || stub.updated.Title == nil || *stub.updated.Title != title {
		t.Fatalf("update request = %+v", stub.updated)
	}
	if err := runtime.Delete(t.Context(), created.ID); err != nil {
		t.Fatal(err)
	}
	handle, err := runtime.RunNow(t.Context(), "sch_1")
	if err != nil || handle.SessionID != "ses_headless" || handle.RunID != "run_headless" {
		t.Fatalf("RunNow = (%+v, %v)", handle, err)
	}
	want := []string{"create", "update", "delete:sch_created", "run:sch_1"}
	if len(stub.actions) != len(want) {
		t.Fatalf("actions = %v, want %v", stub.actions, want)
	}
	for index := range want {
		if stub.actions[index] != want[index] {
			t.Fatalf("actions = %v, want %v", stub.actions, want)
		}
	}
}

func TestScheduleAdapterProjectsWorkspaceChangeSemantics(t *testing.T) {
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	stub := &scheduleBindingStub{t: t, now: now, keys: make(map[string]struct{})}
	runtime := &Runtime{
		schedules: stub,
		workspaces: &workspaceBindingStub{resolved: &protocol.WorkspaceInfo{
			Ref:          protocol.WorkspaceRef{Path: "/workspace"},
			ProjectRoot:  "/workspace",
			Availability: protocol.WorkspaceAvailable,
		}},
		meta: requestMeta("test"),
	}

	_, err := runtime.Update(t.Context(), schedule.Patch{
		ID: "sch_1", ExpectedRevision: 1, Workspace: schedule.BindWorkspace("/workspace/alias"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stub.updated.Workspace == nil || stub.updated.Workspace.Path != "/workspace" || stub.updated.WorkspaceMode != "" {
		t.Fatalf("bound workspace request = %+v", stub.updated)
	}

	_, err = runtime.Update(t.Context(), schedule.Patch{
		ID: "sch_1", ExpectedRevision: 2, Workspace: schedule.UseDefaultWorkspace(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stub.updated.Workspace != nil || stub.updated.WorkspaceMode != protocol.ScheduleWorkspaceDefault {
		t.Fatalf("default workspace request = %+v", stub.updated)
	}
}

func TestScheduleAdapterRejectsAMutationForAnotherSchedule(t *testing.T) {
	t.Parallel()
	value := wireSchedule(time.Unix(1, 0), "sch_other")
	_, err := projectScheduleResult("update schedule", "sch_expected", &value, nil)
	requireRuntimeContractViolation(t, err)
}

func TestScheduleAdapterRejectsMutationAcknowledgementDrift(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	candidate := schedule.Candidate{Title: "Review", Instructions: "review", Cron: "0 * * * *"}
	wrongCreate := wireSchedule(now, "sch_created")
	wrongCreate.Instructions = "ignored"
	wrongUpdate := wireSchedule(now, "sch_1")
	wrongUpdate.Revision = 2
	wrongUpdate.Title = "ignored"
	tests := []struct {
		name   string
		stub   *scheduleBindingStub
		invoke func(*Runtime) error
	}{
		{
			name: "create fields",
			stub: &scheduleBindingStub{now: now, createResult: &wrongCreate},
			invoke: func(runtime *Runtime) error {
				_, err := runtime.Create(t.Context(), candidate)
				return err
			},
		},
		{
			name: "update fields",
			stub: &scheduleBindingStub{now: now, updateResult: &wrongUpdate},
			invoke: func(runtime *Runtime) error {
				title := "updated"
				_, err := runtime.Update(t.Context(), schedule.Patch{
					ID: "sch_1", ExpectedRevision: 1, Title: &title,
				})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.stub.t = t
			test.stub.keys = make(map[string]struct{})
			runtime := &Runtime{schedules: test.stub, meta: requestMeta("test")}
			requireRuntimeContractViolation(t, test.invoke(runtime))
		})
	}
}
