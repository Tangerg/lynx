package agentexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
)

// --- fakes -------------------------------------------------------------------

type fakeOffloader struct {
	calls    int
	lastArgs [3]string // session, tool, body
	id       string
	err      error
}

func (f *fakeOffloader) Offload(_ context.Context, session, tool, body string) (string, error) {
	f.calls++
	f.lastArgs = [3]string{session, tool, body}
	return f.id, f.err
}

type fakeTool struct {
	name       string
	output     string
	err        error
	concKey    string
	concurrent bool
	direct     bool
	mutations  []string
}

func (t *fakeTool) Definition() chat.ToolDefinition              { return chat.ToolDefinition{Name: t.name} }
func (t *fakeTool) Call(context.Context, string) (string, error) { return t.output, t.err }
func (t *fakeTool) ConcurrencyKey(string) (string, bool)         { return t.concKey, t.concurrent }
func (t *fakeTool) ReturnsDirect() bool                          { return t.direct }
func (t *fakeTool) MutationPaths(string) ([]string, error)       { return t.mutations, nil }

// fakeBlackboard / fakeProcessView satisfy just enough of the agent SDK's
// read interfaces (via embedding) for turnctx.TurnSession to resolve a session
// off ctx — the rest of the surface is never called on this path.
type fakeBlackboard struct {
	core.BlackboardReader
	vals map[string]any
}

func (b fakeBlackboard) Load(key string) (any, bool) { v, ok := b.vals[key]; return v, ok }

type fakeProcessView struct {
	core.ProcessView
	bb core.BlackboardReader
}

func (p fakeProcessView) Blackboard() core.BlackboardReader { return p.bb }

func sessionCtx(session string) context.Context {
	bb := fakeBlackboard{vals: map[string]any{turnctx.SessionBindingKey: session}}
	return core.WithProcessView(context.Background(), fakeProcessView{bb: bb})
}

// --- tests -------------------------------------------------------------------

func TestEviction_OversizedIsOffloadedWithRetrievablePlaceholder(t *testing.T) {
	store := &fakeOffloader{id: "blob-123"}
	mw := &toolResultEvictionMiddleware{store: store, threshold: 100}
	body := strings.Repeat("x", 500)
	tool := mw.WrapTool(nil, nil, &fakeTool{name: "shell", output: body})

	got, err := tool.Call(sessionCtx("sess-1"), "{}")
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if store.calls != 1 {
		t.Fatalf("Offload called %d times, want 1", store.calls)
	}
	if store.lastArgs != [3]string{"sess-1", "shell", body} {
		t.Fatalf("Offload args = %v, want session/tool/full-body", store.lastArgs)
	}
	if len(got) >= len(body) {
		t.Fatalf("placeholder (%d bytes) is not smaller than the body (%d)", len(got), len(body))
	}
	for _, want := range []string{"blob-123", toolport.ToolNameReadToolResult, "500 chars"} {
		if !strings.Contains(got, want) {
			t.Errorf("placeholder missing %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "xxx") {
		t.Errorf("placeholder dropped the head preview: %q", got[:min(20, len(got))])
	}
}

func TestEviction_SmallResultUntouched(t *testing.T) {
	store := &fakeOffloader{id: "unused"}
	mw := &toolResultEvictionMiddleware{store: store, threshold: 100}
	tool := mw.WrapTool(nil, nil, &fakeTool{name: "shell", output: "small"})

	got, err := tool.Call(sessionCtx("s"), "{}")
	if err != nil || got != "small" {
		t.Fatalf("Call = (%q, %v), want (small, nil)", got, err)
	}
	if store.calls != 0 {
		t.Fatal("a below-threshold result must not be offloaded")
	}
}

func TestEviction_ToolErrorPassesThroughUnoffloaded(t *testing.T) {
	store := &fakeOffloader{}
	mw := &toolResultEvictionMiddleware{store: store, threshold: 10}
	sentinel := errors.New("boom")
	tool := mw.WrapTool(nil, nil, &fakeTool{name: "shell", output: strings.Repeat("x", 500), err: sentinel})

	got, err := tool.Call(sessionCtx("s"), "{}")
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the tool's error", err)
	}
	if got == "" || store.calls != 0 {
		t.Fatalf("a failed call must return its output verbatim and not offload (got %q, calls %d)", got, store.calls)
	}
}

func TestEviction_NoSessionKeepsFullBody(t *testing.T) {
	store := &fakeOffloader{id: "x"}
	mw := &toolResultEvictionMiddleware{store: store, threshold: 10}
	body := strings.Repeat("x", 500)
	tool := mw.WrapTool(nil, nil, &fakeTool{name: "shell", output: body})

	// Bare ctx → turnctx.TurnSession == "" → nothing to scope the blob under.
	got, err := tool.Call(context.Background(), "{}")
	if err != nil || got != body {
		t.Fatalf("no-session oversized Call = (len %d, %v), want the full body unchanged", len(got), err)
	}
	if store.calls != 0 {
		t.Fatal("must not offload without a session to scope/retrieve the blob")
	}
}

func TestEviction_OffloadFailureDegradesToFullBody(t *testing.T) {
	store := &fakeOffloader{err: errors.New("db down")}
	mw := &toolResultEvictionMiddleware{store: store, threshold: 10}
	body := strings.Repeat("x", 500)
	tool := mw.WrapTool(nil, nil, &fakeTool{name: "shell", output: body})

	got, err := tool.Call(sessionCtx("s"), "{}")
	if err != nil {
		t.Fatalf("a failed offload must not fail the call: %v", err)
	}
	if got != body {
		t.Fatal("a failed offload must degrade to the full body, not a broken placeholder")
	}
}

func TestEviction_WrapToolExcludesReadBackTool(t *testing.T) {
	mw := &toolResultEvictionMiddleware{store: &fakeOffloader{}, threshold: 10}

	readBack := &fakeTool{name: toolport.ToolNameReadToolResult}
	if got := mw.WrapTool(nil, nil, readBack); got != readBack {
		t.Fatal("the read-back tool must be returned unwrapped (evicting its output would loop)")
	}
	if _, ok := mw.WrapTool(nil, nil, &fakeTool{name: "shell"}).(*evictingTool); !ok {
		t.Fatal("a normal tool must be wrapped in evictingTool")
	}
}

func TestEvictingTool_ForwardsCapabilities(t *testing.T) {
	mw := &toolResultEvictionMiddleware{store: &fakeOffloader{}, threshold: 10}
	inner := &fakeTool{name: "shell", concKey: "res-A", concurrent: true, direct: true, mutations: []string{"a.go"}}
	wrapped := mw.WrapTool(nil, nil, inner)

	if key, conc := wrapped.(interface {
		ConcurrencyKey(string) (string, bool)
	}).ConcurrencyKey("{}"); key != "res-A" || !conc {
		t.Errorf("ConcurrencyKey = (%q, %v), want forwarded (res-A, true)", key, conc)
	}
	if !wrapped.(interface{ ReturnsDirect() bool }).ReturnsDirect() {
		t.Error("ReturnsDirect not forwarded")
	}
	paths, _ := wrapped.(interface {
		MutationPaths(string) ([]string, error)
	}).MutationPaths("{}")
	if len(paths) != 1 || paths[0] != "a.go" {
		t.Errorf("MutationPaths = %v, want forwarded [a.go]", paths)
	}
}

func TestNewToolResultEviction_DisabledCases(t *testing.T) {
	if newToolResultEviction(nil, 100) != nil {
		t.Error("nil store must disable eviction")
	}
	if newToolResultEviction(&fakeOffloader{}, 0) != nil {
		t.Error("zero threshold must disable eviction")
	}
	if newToolResultEviction(&fakeOffloader{}, -1) != nil {
		t.Error("negative threshold must disable eviction")
	}
	if newToolResultEviction(&fakeOffloader{}, 100) == nil {
		t.Error("a store with a positive threshold must enable eviction")
	}
}
