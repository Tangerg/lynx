package agentexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/toolport"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turnctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
)

type fakeOffloader struct {
	calls      int
	discards   int
	lastArgs   [3]string // session, tool, body
	discardRef offload.Ref
	id         offload.ID
	err        error
}

func (f *fakeOffloader) Discard(_ context.Context, _ string, ref offload.Ref) error {
	f.discards++
	f.discardRef = ref
	return nil
}

func (f *fakeOffloader) Offload(_ context.Context, session, tool, body string) (offload.ID, error) {
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

func TestEvict_OversizedIsOffloadedWithRetrievablePreview(t *testing.T) {
	store := &fakeOffloader{id: "BLOB234ID"}
	obs := newObservationWith(store, 100)
	body := strings.Repeat("x", 500)

	got, ref := obs.evict(sessionCtx("sess-1"), "shell", body)
	if store.calls != 1 {
		t.Fatalf("Offload called %d times, want 1", store.calls)
	}
	if store.lastArgs != [3]string{"sess-1", "shell", body} {
		t.Fatalf("Offload args = %v, want session/tool/full-body", store.lastArgs)
	}
	if len(got) >= len(body) {
		t.Fatalf("preview (%d) not smaller than body (%d)", len(got), len(body))
	}
	if ref == nil || ref.ID != "BLOB234ID" {
		t.Fatalf("offload ref = %+v, want BLOB234ID", ref)
	}
}

func TestEvict_SmallResultUntouched(t *testing.T) {
	store := &fakeOffloader{}
	obs := newObservationWith(store, 100)
	if got, ref := obs.evict(sessionCtx("s"), "shell", "small"); got != "small" || ref != nil || store.calls != 0 {
		t.Fatalf("small result: got %q, calls %d — want (small, 0)", got, store.calls)
	}
}

func TestEvict_UnprofitablePreviewKeepsBodyAndDiscardsStage(t *testing.T) {
	store := &fakeOffloader{id: "BLOB234ID"}
	obs := newObservationWith(store, 1)
	body := "xx"

	got, ref := obs.evict(sessionCtx("sess-1"), "shell", body)
	if got != body || ref != nil {
		t.Fatalf("unprofitable eviction = (%q, %+v), want original body", got, ref)
	}
	if store.calls != 1 || store.discards != 1 || store.discardRef.ID != "BLOB234ID" {
		t.Fatalf("stage lifecycle = offloads:%d discards:%d ref:%+v", store.calls, store.discards, store.discardRef)
	}
}

func TestEvict_ReadBackToolExcluded(t *testing.T) {
	store := &fakeOffloader{id: "x"}
	obs := newObservationWith(store, 10)
	body := strings.Repeat("x", 500)
	// Evicting the read-back tool's own output would loop.
	if got, ref := obs.evict(sessionCtx("s"), toolport.ToolNameReadToolResult, body); got != body || ref != nil || store.calls != 0 {
		t.Fatalf("read-back tool must not be offloaded (calls %d)", store.calls)
	}
}

func TestEvict_NoSessionKeepsFullBody(t *testing.T) {
	store := &fakeOffloader{id: "x"}
	obs := newObservationWith(store, 10)
	body := strings.Repeat("x", 500)
	// Bare ctx → no session → nothing to scope/retrieve the blob under.
	if got, ref := obs.evict(context.Background(), "shell", body); got != body || ref != nil || store.calls != 0 {
		t.Fatalf("no session must keep the full body (calls %d)", store.calls)
	}
}

func TestEvict_OffloadFailureDegradesToFullBody(t *testing.T) {
	store := &fakeOffloader{err: errors.New("db down")}
	obs := newObservationWith(store, 10)
	body := strings.Repeat("x", 500)
	if got, ref := obs.evict(sessionCtx("s"), "shell", body); got != body || ref != nil {
		t.Fatal("a failed offload must degrade to the full body, not a broken preview")
	}
}

func TestEvict_DisabledWhenNoStoreOrThreshold(t *testing.T) {
	body := strings.Repeat("x", 500)
	if got, ref := newObservationWith(nil, 100).evict(sessionCtx("s"), "shell", body); got != body || ref != nil {
		t.Error("nil store must disable eviction")
	}
	if got, ref := newObservationWith(&fakeOffloader{}, 0).evict(sessionCtx("s"), "shell", body); got != body || ref != nil {
		t.Error("zero threshold must disable eviction")
	}
}
