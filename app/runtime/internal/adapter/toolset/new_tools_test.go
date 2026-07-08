package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

func TestDownloadTool_WritesAndRefusesBlindOverwrite(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(srv.Close)

	tool := newDownloadTool(dir)
	body, err := tool.Call(t.Context(), `{"url":"`+srv.URL+`","file_path":"out/hello.txt"}`)
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	var resp downloadResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Bytes != 5 || resp.ContentType != "text/plain" {
		t.Fatalf("response = %+v", resp)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "out", "hello.txt"))
	if string(got) != "hello" {
		t.Fatalf("file = %q", got)
	}

	if _, err := tool.Call(t.Context(), `{"url":"`+srv.URL+`","file_path":"out/hello.txt"}`); err == nil {
		t.Fatal("second download without overwrite: want error")
	}
	if _, err := tool.Call(t.Context(), `{"url":"`+srv.URL+`","file_path":"out/hello.txt","overwrite":true}`); err != nil {
		t.Fatalf("download with overwrite: %v", err)
	}
}

func TestSourcegraphStream_FoldsMatchesAndProgress(t *testing.T) {
	stream := `event: matches
data: [{"type":"content","repository":"github.com/acme/repo","path":"main.go","commit":"abc","language":"Go","lineMatches":[{"line":"func main() {}","lineNumber":12}]}]

event: progress
data: {"matchCount":1,"durationMs":42}

event: done
data: {}

`
	out, err := readSourcegraphStream(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if out.MatchCount != 1 || out.DurationMs != 42 || len(out.Matches) != 1 {
		t.Fatalf("stream output = %+v", out)
	}
	if got := out.Matches[0].LineMatches[0].LineNumber; got != 12 {
		t.Fatalf("line number = %d, want 12", got)
	}
}

func TestScheduleTools_CreateUpdateDelete(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tools, err := newScheduleTools(reg)
	if err != nil {
		t.Fatalf("newScheduleTools: %v", err)
	}
	byName := map[string]chat.Tool{}
	for _, tool := range tools {
		byName[tool.Definition().Name] = tool
	}
	create := byName["schedule_create"].(interface {
		Call(context.Context, string) (string, error)
	})
	update := byName["schedule_update"].(interface {
		Call(context.Context, string) (string, error)
	})
	deleteTool := byName["schedule_delete"].(interface {
		Call(context.Context, string) (string, error)
	})

	body, err := create.Call(t.Context(), `{"title":"daily","prompt":"summarize","cron":"0 9 * * *"}`)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	var created scheduleResponse
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	if created.Schedule.ID == "" || created.Schedule.NextRunAt == "" {
		t.Fatalf("created schedule = %+v", created.Schedule)
	}

	if _, err := update.Call(t.Context(), `{"id":"`+created.Schedule.ID+`","enabled":false}`); err != nil {
		t.Fatalf("update: %v", err)
	}
	stored, _ := reg.Get(t.Context(), created.Schedule.ID)
	if stored.Enabled || !stored.NextRunAt.IsZero() {
		t.Fatalf("stored after disable = %+v", stored)
	}

	if _, err := deleteTool.Call(t.Context(), `{"id":"`+created.Schedule.ID+`"}`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := reg.Get(t.Context(), created.Schedule.ID); !errors.Is(err, schedule.ErrNotFound) {
		t.Fatalf("get deleted err = %v, want ErrNotFound", err)
	}
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
	return chat.ToolDefinition{Name: "apply_patch", InputSchema: `{"type":"object"}`}
}

func (p *patchPathStub) Call(context.Context, string) (string, error) {
	*p.called = true
	return "patched", nil
}

func (p *patchPathStub) MutatedPaths(arguments string) ([]string, error) {
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

func (m *memoryScheduleRegistry) Update(_ context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
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

func (m *memoryScheduleRegistry) Due(context.Context, time.Time) ([]schedule.Schedule, error) {
	return nil, nil
}

func (m *memoryScheduleRegistry) MarkFired(context.Context, string, time.Time, time.Time, time.Time) error {
	return nil
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
