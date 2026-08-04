package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/workspacepath"
	scheduleapp "github.com/Tangerg/lynx/app/runtime/internal/application/schedules"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

func TestFormatJSON_WritesIndentedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.json")
	if err := os.WriteFile(path, []byte(`{"b":1,"a":2}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := formatJSON(path, 0o600); err != nil {
		t.Fatalf("formatJSON: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	want := "{\n  \"b\": 1,\n  \"a\": 2\n}\n"
	if string(got) != want {
		t.Fatalf("formatted JSON = %q, want %q", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestSchedulesCreateListDelete(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := buildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("buildSchedules: %v", err)
	}
	byName := toolNameMap(tools)

	body, err := byName["create_schedule"].Call(t.Context(), `{"title":"daily","instructions":"summarize","cron":"0 9 * * *"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created scheduleResponse
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
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
	if _, err := reg.Get(t.Context(), created.Schedule.ScheduleID); !errors.Is(err, schedule.ErrNotFound) {
		t.Fatalf("get deleted err = %v, want ErrNotFound", err)
	}
}

func TestSchedulesHaveActionSpecificStrictSchemas(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := buildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("buildSchedules: %v", err)
	}
	byName := toolNameMap(tools)
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
	tools, err := buildSchedules(newTestScheduleCoordinator(reg))
	if err != nil {
		t.Fatalf("buildSchedules: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	_, err = toolNameMap(tools)["create_schedule"].Call(t.Context(), `{"instructions":"summarize","cron":"0 9 * * *","workdir":"`+missing+`"}`)
	if !errors.Is(err, schedule.ErrCwdUnavailable) {
		t.Fatalf("create cwd err = %v, want ErrCwdUnavailable", err)
	}
	if len(reg.items) != 0 {
		t.Fatalf("created %d schedule(s), want none", len(reg.items))
	}
}

func toolNameMap(tools []toolcontract.Tool) map[string]toolcontract.Tool {
	out := make(map[string]toolcontract.Tool, len(tools))
	for _, candidate := range tools {
		out[candidate.Definition().Name] = candidate
	}
	return out
}

func TestPathGuard_ApplyPatchChecksAllTargets(t *testing.T) {
	called := false
	tool := withPathGuard(&patchPathStub{called: &called}, "/work")
	patch := `--- a/ok.txt
+++ b/ok.txt
@@ -1 +1 @@
-old
+new
--- a/.git/config
+++ b/.git/config
@@ -1 +1 @@
-old
+new
`
	out, err := tool.Call(t.Context(), string(mustMarshal(t, map[string]string{"patch": patch})))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if called {
		t.Fatal("inner tool ran despite protected path in patch")
	}
	if !strings.Contains(out, "Refused") {
		t.Fatalf("out = %q, want refusal", out)
	}
}

type patchPathStub struct {
	called *bool
}

func (p *patchPathStub) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: "apply_patch", InputSchema: json.RawMessage(`{"type":"object"}`)}
}

func (p *patchPathStub) Call(context.Context, string) (string, error) {
	*p.called = true
	return "patched", nil
}

func (p *patchPathStub) MutationPaths(arguments string) ([]string, error) {
	var req struct {
		Patch string `json:"patch"`
	}
	_ = json.Unmarshal([]byte(arguments), &req)
	return []string{"ok.txt", ".git/config"}, nil
}

type memoryScheduleRegistry struct {
	items map[string]schedule.Schedule
	next  int
}

func newMemoryScheduleRegistry() *memoryScheduleRegistry {
	return &memoryScheduleRegistry{items: map[string]schedule.Schedule{}}
}

func newTestScheduleCoordinator(reg scheduleapp.ManagementStore) *scheduleapp.Coordinator {
	return scheduleapp.New(scheduleapp.Dependencies{
		Store: reg,
		Paths: workspacepath.Resolver{},
	})
}

func (m *memoryScheduleRegistry) ListPage(ctx context.Context, _ int64, _ string, _ int) ([]schedule.Schedule, error) {
	return m.List(ctx)
}

func (m *memoryScheduleRegistry) List(context.Context) ([]schedule.Schedule, error) {
	out := make([]schedule.Schedule, 0, len(m.items))
	for _, sc := range m.items {
		out = append(out, sc)
	}
	return out, nil
}

func (m *memoryScheduleRegistry) Get(_ context.Context, id string) (schedule.Schedule, error) {
	sc, ok := m.items[id]
	if !ok {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	return sc, nil
}

func (m *memoryScheduleRegistry) Create(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	m.next++
	sc.ID = fmt.Sprintf("sch_test_%d", m.next)
	sc.CreatedAt = time.Now().UTC()
	m.items[sc.ID] = sc
	return sc, nil
}

func (m *memoryScheduleRegistry) Update(_ context.Context, sc schedule.Schedule, _ uint64) (schedule.Schedule, error) {
	if _, ok := m.items[sc.ID]; !ok {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	m.items[sc.ID] = sc
	return sc, nil
}

func (m *memoryScheduleRegistry) Delete(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}

func (m *memoryScheduleRegistry) Due(context.Context, time.Time, int) ([]schedule.Schedule, error) {
	return nil, nil
}

func (m *memoryScheduleRegistry) RecordRun(context.Context, string, time.Time) error {
	return nil
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}
