package offload

import (
	"context"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	resultoffload "github.com/Tangerg/lynx/app/runtime/internal/domain/execution/offload"
)

type fakeStore struct {
	body        string
	found       bool
	err         error
	lastSession string
	lastID      resultoffload.ID
}

func (f *fakeStore) Fetch(_ context.Context, session string, id resultoffload.ID) (string, bool, error) {
	f.lastSession, f.lastID = session, id
	return f.body, f.found, f.err
}

func sessionCtx(session string) context.Context {
	return executionctx.WithScope(context.Background(), execution.ExecutionScope{SessionID: session})
}

func TestNew_NilStoreOmitted(t *testing.T) {
	tool, err := New(nil)
	if err != nil {
		t.Fatal(err)
	}
	if tool != nil {
		t.Fatal("New(nil) should return nil so the caller omits the tool (eviction disabled)")
	}
}

func TestNewUsesCanonicalName(t *testing.T) {
	tool, err := New(&fakeStore{})
	if err != nil || tool == nil {
		t.Fatalf("New = (%v, %v), want a tool", tool, err)
	}
	if got := tool.Definition().Name; got != Name {
		t.Fatalf("tool name = %q, want %q", got, Name)
	}
}

func TestRead_ReturnsStoredBody(t *testing.T) {
	store := &fakeStore{body: "ABCDEFGHIJ", found: true}
	tool, _ := New(store)
	out, err := tool.Call(sessionCtx("sess-1"), `{"result_id":"BLOB234"}`)
	if err != nil {
		t.Fatal(err)
	}
	if store.lastSession != "sess-1" || store.lastID != "BLOB234" {
		t.Fatalf("Fetch called with (%q, %q), want (sess-1, BLOB234)", store.lastSession, store.lastID)
	}
	if !strings.Contains(out, "ABCDEFGHIJ") || !strings.Contains(out, "10 bytes total") {
		t.Fatalf("output missing body or total:\n%s", out)
	}
}

func TestRead_PagesWithOffsetAndLimit(t *testing.T) {
	store := &fakeStore{body: "ABCDEFGHIJ", found: true}
	tool, _ := New(store)
	out, err := tool.Call(sessionCtx("s"), `{"result_id":"ABCDE234","offset_bytes":2,"limit_bytes":3}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "CDE") || strings.Contains(out, "ABCDEFGHIJ") {
		t.Fatalf("windowed read did not return exactly the slice:\n%s", out)
	}
	// 5 < 10 → the tool should tell the model more remains and where to resume.
	if !strings.Contains(out, "remain") || !strings.Contains(out, `"offset_bytes":5`) {
		t.Fatalf("missing continuation hint:\n%s", out)
	}
}

func TestRead_UnknownIDIsRecoverable(t *testing.T) {
	tool, _ := New(&fakeStore{found: false})
	out, err := tool.Call(sessionCtx("s"), `{"result_id":"NOPE234"}`)
	if err != nil {
		t.Fatalf("an unknown id must not error: %v", err)
	}
	if !strings.Contains(out, "No stored tool result") {
		t.Fatalf("output = %q, want a not-found message", out)
	}
}

func TestRead_EmptyResultIDRejected(t *testing.T) {
	tool, _ := New(&fakeStore{})
	if _, err := tool.Call(sessionCtx("s"), `{"result_id":""}`); err == nil {
		t.Fatal("expected an error for an empty result_id")
	}
}

func TestRead_InvalidIDRejectedBeforeStore(t *testing.T) {
	store := new(fakeStore)
	tool, _ := New(store)
	if _, err := tool.Call(sessionCtx("s"), `{"result_id":"not-valid"}`); err == nil || store.lastID != "" {
		t.Fatalf("invalid result_id error = %v, store id = %q", err, store.lastID)
	}
}

func TestRead_NoSession(t *testing.T) {
	tool, _ := New(&fakeStore{})
	out, err := tool.Call(context.Background(), `{"result_id":"ABCDE234"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no active session") {
		t.Fatalf("output = %q, want a no-session message", out)
	}
}

func TestRead_RejectsRemovedAndOversizedPagingArguments(t *testing.T) {
	tool, _ := New(&fakeStore{})
	for _, arguments := range []string{
		`{"id":"ABCDE234"}`,
		`{"result_id":"ABCDE234","offset":1}`,
		`{"result_id":"ABCDE234","limit":1}`,
		`{"result_id":"ABCDE234","limit_bytes":20001}`,
	} {
		if _, err := tool.Call(sessionCtx("s"), arguments); err == nil {
			t.Fatalf("read_tool_result accepted invalid arguments: %s", arguments)
		}
	}
}

func TestWindow(t *testing.T) {
	tests := []struct {
		name               string
		body               string
		offset, limit      int
		wantStart, wantEnd int
	}{
		{"slice", "ABCDEFGHIJ", 2, 3, 2, 5},
		{"offset past end clamps", "ABC", 10, 5, 3, 3},
		{"negative offset floored", "ABC", -1, 2, 0, 2},
		{"zero limit uses default window", "ABCDE", 0, 0, 0, 5},
		{"limit past end clamps", "ABCDE", 1, 99, 1, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := window(tt.body, tt.offset, tt.limit)
			if start != tt.wantStart || end != tt.wantEnd {
				t.Fatalf("window = (%d, %d), want (%d, %d)", start, end, tt.wantStart, tt.wantEnd)
			}
		})
	}
}

func TestWindowSnapsToRuneBoundaries(t *testing.T) {
	body := "aé b" // 'é' is 2 bytes (0xC3 0xA9): bytes are a[0] é[1,2] space[3] b[4]
	// An offset landing inside 'é' (byte 2) must snap forward to the next rune
	// start (byte 3) so the returned slice is never a broken rune.
	start, _ := window(body, 2, 1)
	if start != 3 {
		t.Fatalf("start = %d, want 3 (snapped past the mid-rune byte)", start)
	}
	// A limit ending inside 'é' must snap the end forward too.
	_, end := window(body, 0, 2)
	if end != 3 {
		t.Fatalf("end = %d, want 3 (snapped past the mid-rune byte)", end)
	}
}
