package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	toolcontract "github.com/Tangerg/lynx/core/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	workspaceapp "github.com/Tangerg/lynx/app/runtime/internal/application/workspace"
	scheduledomain "github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

func TestSchedulesCreateListDelete(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := BuildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byName := scheduleByName(tools)

	body, err := byName["create_schedule"].Call(t.Context(), `{"title":"daily","instructions":"summarize","cron":"0 9 * * *"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created scheduleResponse
	if unmarshalErr := json.Unmarshal([]byte(body), &created); unmarshalErr != nil {
		t.Fatalf("unmarshal create: %v", unmarshalErr)
	}
	if created.Schedule.ScheduleID == "" || created.Schedule.NextRunAt == "" || created.Schedule.Instructions != "summarize" {
		t.Fatalf("created schedule = %+v", created.Schedule)
	}

	listBody, err := byName["list_schedules"].Call(t.Context(), `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var listed scheduleListResponse
	if err := json.Unmarshal([]byte(listBody), &listed); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listed.Schedules) != 1 {
		t.Fatalf("list = %+v, want 1 schedule", listed.Schedules)
	}

	if _, err := byName["delete_schedule"].Call(t.Context(), `{"schedule_id":"`+created.Schedule.ScheduleID+`"}`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := reg.Get(t.Context(), created.Schedule.ScheduleID); !errors.Is(err, scheduledomain.ErrNotFound) {
		t.Fatalf("get deleted err = %v, want ErrNotFound", err)
	}
}

func TestSchedulesHaveActionSpecificStrictSchemas(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := BuildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	byName := scheduleByName(tools)
	if _, err := byName["create_schedule"].Call(t.Context(), `{"cron":"0 9 * * *"}`); err == nil {
		t.Fatal("create without instructions succeeded")
	}
	if _, err := byName["list_schedules"].Call(t.Context(), `{"op":"list"}`); err == nil {
		t.Fatal("list accepted an obsolete op field")
	}
	if _, err := byName["delete_schedule"].Call(t.Context(), `{"id":"sch_old"}`); err == nil {
		t.Fatal("delete accepted obsolete id field")
	}
}

func TestSchedulesCreateRejectsUnavailableWorkdir(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := BuildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	_, err = scheduleByName(tools)["create_schedule"].Call(t.Context(), `{"instructions":"summarize","cron":"0 9 * * *","workspace_path":"`+missing+`"}`)
	if !errors.Is(err, workspaceapp.ErrCWDUnavailable) {
		t.Fatalf("create cwd err = %v, want ErrCWDUnavailable", err)
	}
	if len(reg.items) != 0 {
		t.Fatalf("created %d schedule(s), want none", len(reg.items))
	}
}

func scheduleByName(tools []toolcontract.Tool) map[string]toolcontract.Tool {
	out := make(map[string]toolcontract.Tool, len(tools))
	for _, candidate := range tools {
		out[candidate.Definition().Name] = candidate
	}
	return out
}

type memoryScheduleRegistry struct {
	items map[string]scheduledomain.Schedule
	next  int
}

func newMemoryScheduleRegistry() *memoryScheduleRegistry {
	return &memoryScheduleRegistry{items: map[string]scheduledomain.Schedule{}}
}

func newTestScheduleCoordinator(reg scheduleapp.ManagementStore) *scheduleapp.Coordinator {
	return scheduleapp.New(scheduleapp.Dependencies{
		Store: reg,
		Paths: workspacepath.Resolver{},
	})
}

func (m *memoryScheduleRegistry) ListPage(ctx context.Context, _ time.Time, _ string, _ int) ([]scheduledomain.Schedule, error) {
	return m.List(ctx)
}

func (m *memoryScheduleRegistry) List(context.Context) ([]scheduledomain.Schedule, error) {
	out := make([]scheduledomain.Schedule, 0, len(m.items))
	for _, sc := range m.items {
		out = append(out, sc)
	}
	return out, nil
}

func (m *memoryScheduleRegistry) Get(_ context.Context, id string) (scheduledomain.Schedule, error) {
	sc, ok := m.items[id]
	if !ok {
		return scheduledomain.Schedule{}, scheduledomain.ErrNotFound
	}
	return sc, nil
}

func (m *memoryScheduleRegistry) Create(_ context.Context, sc scheduledomain.Schedule) (scheduledomain.Schedule, error) {
	m.next++
	sc.ID = fmt.Sprintf("sch_test_%d", m.next)
	sc.CreatedAt = time.Now().UTC()
	m.items[sc.ID] = sc
	return sc, nil
}

func (m *memoryScheduleRegistry) Update(_ context.Context, sc scheduledomain.Schedule, _ uint64) (scheduledomain.Schedule, error) {
	if _, ok := m.items[sc.ID]; !ok {
		return scheduledomain.Schedule{}, scheduledomain.ErrNotFound
	}
	m.items[sc.ID] = sc
	return sc, nil
}

func (m *memoryScheduleRegistry) Delete(_ context.Context, id string) (bool, error) {
	_, found := m.items[id]
	delete(m.items, id)
	return found, nil
}

func (m *memoryScheduleRegistry) Due(context.Context, time.Time, int) ([]scheduledomain.Schedule, error) {
	return nil, nil
}

func (m *memoryScheduleRegistry) RecordRun(context.Context, string, time.Time) error {
	return nil
}
