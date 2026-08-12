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
	t       *testing.T
	now     time.Time
	actions []string
	keys    map[string]struct{}
	created protocol.CreateScheduleRequest
	updated protocol.UpdateScheduleRequest
}

func (stub *scheduleBindingStub) ListSchedules(_ context.Context, query protocol.PageQuery, options embedded.CallOptions) (*protocol.Page[protocol.Schedule], error) {
	stub.assertMeta(options.RequestMeta)
	if query.Limit != schedulePageLimit {
		stub.t.Fatalf("schedule page limit = %d", query.Limit)
	}
	first := wireSchedule(stub.now, "sch_1")
	if query.Cursor == "" {
		return protocol.NewPageWithCursor([]protocol.Schedule{first}, "next"), nil
	}
	second := wireSchedule(stub.now.Add(time.Minute), "sch_2")
	return protocol.NewPage([]protocol.Schedule{second}), nil
}

func (stub *scheduleBindingStub) CreateSchedule(_ context.Context, request protocol.CreateScheduleRequest, options embedded.CommandOptions) (*protocol.Schedule, error) {
	stub.assertCommand("create", options)
	stub.created = request
	created := wireSchedule(stub.now, "sch_created")
	created.Title, created.Instructions, created.Cron = request.Title, request.Instructions, request.Cron
	created.Provider, created.Model = request.Provider, request.Model
	created.Workspace = request.Workspace
	return &created, nil
}

func (stub *scheduleBindingStub) UpdateSchedule(_ context.Context, request protocol.UpdateScheduleRequest, options embedded.CommandOptions) (*protocol.Schedule, error) {
	stub.assertCommand("update", options)
	stub.updated = request
	updated := wireSchedule(stub.now, request.ID)
	updated.Revision = request.ExpectedRevision + 1
	if request.Title != nil {
		updated.Title = *request.Title
	}
	return &updated, nil
}

func (stub *scheduleBindingStub) DeleteSchedule(_ context.Context, request protocol.DeleteScheduleRequest, options embedded.CommandOptions) error {
	stub.assertCommand("delete:"+request.ID, options)
	return nil
}

func (stub *scheduleBindingStub) RunScheduleNow(_ context.Context, request protocol.RunScheduleNowRequest, options embedded.CommandOptions) (*protocol.RunScheduleNowResponse, error) {
	stub.assertCommand("run:"+request.ID, options)
	return &protocol.RunScheduleNowResponse{SessionID: "ses_headless", RunID: "run_headless"}, nil
}

func (stub *scheduleBindingStub) assertMeta(meta protocol.RequestMeta) {
	stub.t.Helper()
	if meta.ProtocolVersion != protocol.ProtocolVersion {
		stub.t.Fatalf("schedule request meta = %+v", meta)
	}
}

func (stub *scheduleBindingStub) assertCommand(action string, options embedded.CommandOptions) {
	stub.t.Helper()
	stub.assertMeta(options.RequestMeta)
	if options.IdempotencyKey == "" {
		stub.t.Fatal("schedule command has no idempotency key")
	}
	if _, duplicate := stub.keys[options.IdempotencyKey]; duplicate {
		stub.t.Fatalf("schedule command reused idempotency key %q", options.IdempotencyKey)
	}
	stub.keys[options.IdempotencyKey] = struct{}{}
	stub.actions = append(stub.actions, action)
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
	runtime := &Runtime{schedules: stub, meta: requestMeta("test")}

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
	runtime := &Runtime{schedules: stub, meta: requestMeta("test")}

	_, err := runtime.Update(t.Context(), schedule.Patch{
		ID: "sch_1", ExpectedRevision: 1, Workspace: schedule.BindWorkspace("/workspace"),
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
