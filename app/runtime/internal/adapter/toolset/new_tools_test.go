package toolset

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools/httpreq"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

func TestDownloadTool_WritesAndRefusesBlindOverwrite(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello"))
	}))
	t.Cleanup(srv.Close)

	srvURL, _ := url.Parse(srv.URL)
	allow, err := httpreq.NewAllowlist([]string{srvURL.Hostname()})
	if err != nil {
		t.Fatalf("allowlist: %v", err)
	}
	tool := newDownloadTool(dir, allow)
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
	body, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	if strings.Contains(string(body), "lineMatches") || strings.Contains(string(body), "lineNumber") {
		t.Fatalf("output kept Sourcegraph camelCase fields: %s", body)
	}
	if !strings.Contains(string(body), "line_matches") || !strings.Contains(string(body), "line_number") {
		t.Fatalf("output missing model-facing line fields: %s", body)
	}
}

func TestSourcegraphStream_ReturnsMalformedEventError(t *testing.T) {
	stream := `event: matches
data: not-json

`
	_, err := readSourcegraphStream(strings.NewReader(stream))
	if err == nil {
		t.Fatal("malformed Sourcegraph event: want error")
	}
	if !strings.Contains(err.Error(), "parse matches event") {
		t.Fatalf("err = %v, want parse matches event", err)
	}
}

func TestSourcegraphStreamURL_NormalizesEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     string
	}{
		{"host only", "https://sourcegraph.example.com", "https://sourcegraph.example.com/.api/search/stream"},
		{"stream path", "https://sourcegraph.example.com/.api/search/stream", "https://sourcegraph.example.com/.api/search/stream"},
		{"stream path trailing slash", "https://sourcegraph.example.com/.api/search/stream/", "https://sourcegraph.example.com/.api/search/stream"},
		{"mounted path", "https://sourcegraph.example.com/sg", "https://sourcegraph.example.com/sg/.api/search/stream"},
		{"drops query", "https://sourcegraph.example.com?x=1", "https://sourcegraph.example.com/.api/search/stream"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sourcegraphStreamURL(tt.endpoint)
			if err != nil {
				t.Fatalf("sourcegraphStreamURL: %v", err)
			}
			if got != tt.want {
				t.Fatalf("stream URL = %q, want %q", got, tt.want)
			}
		})
	}
}

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

func TestScheduleTool_CreateListUpdateDelete(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tool, err := newScheduleTool(reg)
	if err != nil {
		t.Fatalf("newScheduleTool: %v", err)
	}

	body, err := tool.Call(t.Context(), `{"op":"create","title":"daily","prompt":"summarize","cron":"0 9 * * *"}`)
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

	listBody, err := tool.Call(t.Context(), `{"op":"list"}`)
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

	if _, err := tool.Call(t.Context(), `{"op":"update","id":"`+created.Schedule.ID+`","enabled":false}`); err != nil {
		t.Fatalf("update: %v", err)
	}
	stored, _ := reg.Get(t.Context(), created.Schedule.ID)
	if stored.Enabled || !stored.NextRunAt.IsZero() {
		t.Fatalf("stored after disable = %+v", stored)
	}

	if _, err := tool.Call(t.Context(), `{"op":"delete","id":"`+created.Schedule.ID+`"}`); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := reg.Get(t.Context(), created.Schedule.ID); !errors.Is(err, schedule.ErrNotFound) {
		t.Fatalf("get deleted err = %v, want ErrNotFound", err)
	}
}

func TestScheduleTool_CreateValidatesEntityBeforeNextRun(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tool, err := newScheduleTool(reg)
	if err != nil {
		t.Fatalf("newScheduleTool: %v", err)
	}
	_, err = tool.Call(t.Context(), `{"op":"create","cron":"not a cron"}`)
	if !errors.Is(err, schedule.ErrPromptRequired) {
		t.Fatalf("create err = %v, want ErrPromptRequired", err)
	}
}

func TestScheduleTool_UnknownOp(t *testing.T) {
	reg := newMemoryScheduleRegistry()
	tool, _ := newScheduleTool(reg)
	if _, err := tool.Call(t.Context(), `{"op":"nope"}`); err == nil {
		t.Fatal("unknown op: want error")
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
	return chat.ToolDefinition{Name: "apply_patch", InputSchema: json.RawMessage(`{"type":"object"}`)}
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
