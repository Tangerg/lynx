package agentexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/component/offload"
)

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

// fakeBlackboard / fakeProcessView satisfy just enough of the agent SDK's read
// interfaces (via embedding) for turnctx.TurnSession to resolve a session off
// ctx; the rest of the surface is never touched on this path.
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

func newObservationWith(store toolResultOffloader, threshold int) *toolObservation {
	return newToolObservation(noopObserver{}, store, threshold)
}

func TestEvict_OversizedIsOffloadedWithRetrievablePlaceholder(t *testing.T) {
	store := &fakeOffloader{id: "BLOB234ID"}
	obs := newObservationWith(store, 100)
	body := strings.Repeat("x", 500)

	got := obs.evict(sessionCtx("sess-1"), "shell", body)
	if store.calls != 1 {
		t.Fatalf("Offload called %d times, want 1", store.calls)
	}
	if store.lastArgs != [3]string{"sess-1", "shell", body} {
		t.Fatalf("Offload args = %v, want session/tool/full-body", store.lastArgs)
	}
	if len(got) >= len(body) {
		t.Fatalf("placeholder (%d) not smaller than body (%d)", len(got), len(body))
	}
	id, ok := offload.ID(got)
	if !ok || id != "BLOB234ID" {
		t.Fatalf("placeholder id = (%q,%v), want the offloaded id", id, ok)
	}
}

func TestEvict_SmallResultUntouched(t *testing.T) {
	store := &fakeOffloader{}
	obs := newObservationWith(store, 100)
	if got := obs.evict(sessionCtx("s"), "shell", "small"); got != "small" || store.calls != 0 {
		t.Fatalf("small result: got %q, calls %d — want (small, 0)", got, store.calls)
	}
}

func TestEvict_ReadBackToolExcluded(t *testing.T) {
	store := &fakeOffloader{id: "x"}
	obs := newObservationWith(store, 10)
	body := strings.Repeat("x", 500)
	// Evicting the read-back tool's own output would loop.
	if got := obs.evict(sessionCtx("s"), toolport.ToolNameReadToolResult, body); got != body || store.calls != 0 {
		t.Fatalf("read-back tool must not be offloaded (calls %d)", store.calls)
	}
}

func TestEvict_NoSessionKeepsFullBody(t *testing.T) {
	store := &fakeOffloader{id: "x"}
	obs := newObservationWith(store, 10)
	body := strings.Repeat("x", 500)
	// Bare ctx → no session → nothing to scope/retrieve the blob under.
	if got := obs.evict(context.Background(), "shell", body); got != body || store.calls != 0 {
		t.Fatalf("no session must keep the full body (calls %d)", store.calls)
	}
}

func TestEvict_OffloadFailureDegradesToFullBody(t *testing.T) {
	store := &fakeOffloader{err: errors.New("db down")}
	obs := newObservationWith(store, 10)
	body := strings.Repeat("x", 500)
	if got := obs.evict(sessionCtx("s"), "shell", body); got != body {
		t.Fatal("a failed offload must degrade to the full body, not a broken placeholder")
	}
}

func TestEvict_DisabledWhenNoStoreOrThreshold(t *testing.T) {
	body := strings.Repeat("x", 500)
	if got := newObservationWith(nil, 100).evict(sessionCtx("s"), "shell", body); got != body {
		t.Error("nil store must disable eviction")
	}
	if got := newObservationWith(&fakeOffloader{}, 0).evict(sessionCtx("s"), "shell", body); got != body {
		t.Error("zero threshold must disable eviction")
	}
}
