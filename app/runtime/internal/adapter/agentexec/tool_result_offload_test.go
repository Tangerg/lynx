package agentexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/toolresult"
)

const testToolResultReaderName = "read_tool_result"

type fakeOffloader struct {
	calls     int
	lastStage toolresult.Stage
	err       error
}

func (f *fakeOffloader) Stage(_ context.Context, stage toolresult.Stage) error {
	f.calls++
	f.lastStage = stage
	return f.err
}

func evictForTest(store toolResultOffloader, threshold int, sessionID, toolName, body string) (string, *toolresult.Ref) {
	return evictToolResult(
		context.Background(), store, threshold, testToolResultReaderName,
		sessionID, toolName, body,
	)
}

func TestEvict_OversizedIsOffloadedWithRetrievablePreview(t *testing.T) {
	store := new(fakeOffloader)
	body := strings.Repeat("x", 500)

	got, ref := evictForTest(store, 100, "sess-1", "shell", body)
	if store.calls != 1 {
		t.Fatalf("Stage called %d times, want 1", store.calls)
	}
	if store.lastStage.SessionID != "sess-1" || store.lastStage.ToolName != "shell" || store.lastStage.Body != body {
		t.Fatalf("Stage value = %+v, want session/tool/full-body", store.lastStage)
	}
	if len(got) >= len(body) {
		t.Fatalf("preview (%d) not smaller than body (%d)", len(got), len(body))
	}
	if ref == nil || ref.ID != store.lastStage.ID {
		t.Fatalf("offload ref = %+v, staged ID = %q", ref, store.lastStage.ID)
	}
	if err := ref.Validate(); err != nil {
		t.Fatalf("offload ref: %v", err)
	}
	if !strings.Contains(got, store.lastStage.ID.String()) {
		t.Fatalf("preview %q does not contain staged ID %q", got, store.lastStage.ID)
	}
}

func TestEvict_SmallResultUntouched(t *testing.T) {
	store := &fakeOffloader{}
	if got, ref := evictForTest(store, 100, "s", "shell", "small"); got != "small" || ref != nil || store.calls != 0 {
		t.Fatalf("small result: got %q, calls %d — want (small, 0)", got, store.calls)
	}
}

func TestEvict_UnprofitablePreviewKeepsBodyWithoutStaging(t *testing.T) {
	store := new(fakeOffloader)
	body := "xx"

	got, ref := evictForTest(store, 1, "sess-1", "shell", body)
	if got != body || ref != nil {
		t.Fatalf("unprofitable eviction = (%q, %+v), want original body", got, ref)
	}
	if store.calls != 0 {
		t.Fatalf("unprofitable eviction staged %d blob(s), want none", store.calls)
	}
}

func TestEvict_ReadBackToolExcluded(t *testing.T) {
	store := new(fakeOffloader)
	body := strings.Repeat("x", 500)
	// Evicting the read-back tool's own output would loop.
	if got, ref := evictForTest(store, 10, "s", testToolResultReaderName, body); got != body || ref != nil || store.calls != 0 {
		t.Fatalf("read-back tool must not be offloaded (calls %d)", store.calls)
	}
}

func TestEvict_NoSessionKeepsFullBody(t *testing.T) {
	store := new(fakeOffloader)
	body := strings.Repeat("x", 500)
	// Bare ctx → no session → nothing to scope/retrieve the blob under.
	if got, ref := evictForTest(store, 10, "", "shell", body); got != body || ref != nil || store.calls != 0 {
		t.Fatalf("no session must keep the full body (calls %d)", store.calls)
	}
}

func TestEvict_StageFailureDegradesToFullBody(t *testing.T) {
	store := &fakeOffloader{err: errors.New("db down")}
	body := strings.Repeat("x", 500)
	if got, ref := evictForTest(store, 10, "s", "shell", body); got != body || ref != nil {
		t.Fatal("a failed offload must degrade to the full body, not a broken preview")
	}
}

func TestEvict_DisabledWhenNoStoreOrThreshold(t *testing.T) {
	body := strings.Repeat("x", 500)
	if got, ref := evictForTest(nil, 100, "s", "shell", body); got != body || ref != nil {
		t.Error("nil store must disable eviction")
	}
	if got, ref := evictForTest(&fakeOffloader{}, 0, "s", "shell", body); got != body || ref != nil {
		t.Error("zero threshold must disable eviction")
	}
}
