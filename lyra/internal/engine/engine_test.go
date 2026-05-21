package engine

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

// memoryNewInMemoryStore re-exports memory.NewInMemoryStore under a
// shorter test-only name so the persistent-store test reads as
// "shared store" rather than spelling out the lynx package twice.
func memoryNewInMemoryStore() memory.Store { return memory.NewInMemoryStore() }

// TestEngine_RunChat_ToolCallObserved drives the engine with a stub
// model that asks for a `bash` tool call (echo lyra), then returns a
// final text mentioning the captured output. The observer must see
// one OnToolCallStart / OnToolCallEnd pair; the returned reply must
// be the stub's FinalText.
//
// This is the M2-readiness gate: it proves the chain
// engine.RunChat → lynx Platform → ToolMiddleware → ToolDecorator
// → observedTool → ToolObserver is wired end-to-end without any
// real LLM in the loop.
func TestEngine_RunChat_ToolCallObserved(t *testing.T) {
	stub := newStubModel("bash", `{"command":"echo lyra"}`, "I ran echo and got lyra.")
	client, err := chat.NewClient(stub)
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	eng, err := New(Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	rec := &recordingObserver{}
	reply, err := eng.RunChat(context.Background(), RunChatRequest{
		Message:  "say lyra via bash",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}

	if reply != "I ran echo and got lyra." {
		t.Errorf("reply mismatch: got %q", reply)
	}

	starts := rec.starts()
	ends := rec.ends()

	if len(starts) != 1 {
		t.Fatalf("OnToolCallStart count = %d, want 1; got %#v", len(starts), starts)
	}
	if starts[0].toolName != "bash" {
		t.Errorf("start tool name = %q, want bash", starts[0].toolName)
	}
	if !strings.Contains(starts[0].arguments, "echo lyra") {
		t.Errorf("start arguments missing command: %q", starts[0].arguments)
	}

	if len(ends) != 1 {
		t.Fatalf("OnToolCallEnd count = %d, want 1", len(ends))
	}
	if ends[0].err != nil {
		t.Errorf("end err: %v", ends[0].err)
	}
	if !strings.Contains(ends[0].output, "lyra") {
		t.Errorf("end output missing 'lyra': %q", ends[0].output)
	}
	// Start and end must share the same CallID so observers can pair them.
	if starts[0].callID != ends[0].callID {
		t.Errorf("call id mismatch: start=%s end=%s", starts[0].callID, ends[0].callID)
	}
}

// TestEngine_RunChat_NoObserver verifies the nil-observer path: the
// engine still drives the tool loop, just without firing any
// notifications.
func TestEngine_RunChat_NoObserver(t *testing.T) {
	stub := newStubModel("bash", `{"command":"echo lyra"}`, "done")
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	reply, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go"})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if reply != "done" {
		t.Errorf("reply = %q, want %q", reply, "done")
	}
}

// TestEngine_RunChat_StreamingDeltas verifies the engine forwards
// every chunk the model emits to OnMessageDelta — i.e. text is
// streamed, not buffered. The returned reply is the concatenation
// of all chunks.
func TestEngine_RunChat_StreamingDeltas(t *testing.T) {
	stub := newStreamingStubModel("Hello, ", "world!", " (lyra)")
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	rec := &recordingObserver{}
	reply, err := eng.RunChat(context.Background(), RunChatRequest{
		Message:  "go",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if reply != "Hello, world! (lyra)" {
		t.Errorf("reply = %q, want %q", reply, "Hello, world! (lyra)")
	}

	deltas := rec.deltas()
	wantDeltas := []string{"Hello, ", "world!", " (lyra)"}
	if len(deltas) != len(wantDeltas) {
		t.Fatalf("delta count = %d, want %d (%v)", len(deltas), len(wantDeltas), deltas)
	}
	for i := range deltas {
		if deltas[i] != wantDeltas[i] {
			t.Errorf("delta[%d] = %q, want %q", i, deltas[i], wantDeltas[i])
		}
	}
}

// TestEngine_RunChat_MultiTurnMemory verifies the chat-memory
// middleware loads prior turns before each call. Running two turns
// against the same SessionID must result in the second Call seeing
// strictly more messages than the first (history of turn 1 + new
// user message of turn 2).
func TestEngine_RunChat_MultiTurnMemory(t *testing.T) {
	stub := newHistoryAwareStub()
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	const sessionID = "sess-memory"

	if _, err := eng.RunChat(context.Background(), RunChatRequest{
		SessionID: sessionID,
		Message:   "hello",
	}); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, err := eng.RunChat(context.Background(), RunChatRequest{
		SessionID: sessionID,
		Message:   "again",
	}); err != nil {
		t.Fatalf("turn 2: %v", err)
	}

	if len(stub.seenLengths) < 2 {
		t.Fatalf("seenLengths = %v, want at least 2 entries", stub.seenLengths)
	}
	if stub.seenLengths[1] <= stub.seenLengths[0] {
		t.Errorf("turn 2 should see more messages than turn 1; got %v", stub.seenLengths)
	}
}

// TestEngine_RunChat_PersistentMemoryStoreRoundTrip verifies that a
// caller-supplied [memory.Store] survives engine reconstruction —
// the use case for storage.FileMessageStore + cross-process
// session resume. Two engines built on the same store + same
// SessionID must see history accumulate across instances.
func TestEngine_RunChat_PersistentMemoryStoreRoundTrip(t *testing.T) {
	shared := memoryNewInMemoryStore() // built-in store; durability proxy
	stub1 := newHistoryAwareStub()
	cli1, _ := chat.NewClient(stub1)
	eng1, _ := New(Config{ChatClient: cli1, MemoryStore: shared})

	const sessionID = "shared-sess"
	if _, err := eng1.RunChat(context.Background(), RunChatRequest{
		SessionID: sessionID, Message: "first",
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate process restart: brand-new engine, same store.
	stub2 := newHistoryAwareStub()
	cli2, _ := chat.NewClient(stub2)
	eng2, _ := New(Config{ChatClient: cli2, MemoryStore: shared})

	if _, err := eng2.RunChat(context.Background(), RunChatRequest{
		SessionID: sessionID, Message: "second",
	}); err != nil {
		t.Fatal(err)
	}

	// Second engine's first call should have seen turn-1's history.
	if len(stub2.seenLengths) != 1 {
		t.Fatalf("stub2.seenLengths = %v, want one entry", stub2.seenLengths)
	}
	if stub2.seenLengths[0] <= 1 {
		t.Errorf("second engine should see prior history; got len=%d", stub2.seenLengths[0])
	}
}

// TestEngine_RunChat_NoSessionIDDoesNotPersist verifies turns without
// a SessionID stay isolated — running twice with empty SessionID
// must see identical message counts (no history loaded).
func TestEngine_RunChat_NoSessionIDDoesNotPersist(t *testing.T) {
	stub := newHistoryAwareStub()
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if _, err := eng.RunChat(context.Background(), RunChatRequest{
			Message: "hello",
		}); err != nil {
			t.Fatalf("turn %d: %v", i, err)
		}
	}

	if len(stub.seenLengths) != 2 {
		t.Fatalf("seenLengths = %v, want 2 entries", stub.seenLengths)
	}
	if stub.seenLengths[0] != stub.seenLengths[1] {
		t.Errorf("both turns should see same message count; got %v", stub.seenLengths)
	}
}

// TestEngine_Tools_OfflineOnly verifies the engine exposes the
// always-on coding tool set when no Online credentials are
// configured. Provider-backed tools must NOT appear.
func TestEngine_Tools_OfflineOnly(t *testing.T) {
	stub := newStubModel("bash", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	tools := eng.Tools()
	if len(tools) != 6 {
		t.Fatalf("tool count = %d, want 6 (offline-only)", len(tools))
	}

	names := toolNames(tools)
	for _, want := range []string{"read", "write", "edit", "glob", "grep", "bash"} {
		if !names[want] {
			t.Errorf("missing tool %q in %v", want, names)
		}
	}
	for _, never := range []string{"web_fetch", "web_search", "http_request"} {
		if names[never] {
			t.Errorf("unexpected online tool %q in offline build", never)
		}
	}
}

// TestEngine_Tools_OnlineEnabled verifies provider-backed tools
// arrive when their credentials are supplied.
func TestEngine_Tools_OnlineEnabled(t *testing.T) {
	stub := newStubModel("bash", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{
		ChatClient: client,
		Online: OnlineConfig{
			JinaAPIKey:       "test-jina",
			TavilyAPIKey:     "test-tavily",
			HTTPAllowedHosts: []string{"api.example.com"},
		},
	})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	tools := eng.Tools()
	if len(tools) != 9 {
		t.Fatalf("tool count = %d, want 9 (6 offline + 3 online)", len(tools))
	}
	names := toolNames(tools)
	for _, want := range []string{"web_fetch", "web_search", "http_request"} {
		if !names[want] {
			t.Errorf("expected online tool %q in %v", want, names)
		}
	}
}

// TestEngine_Tools_PartialOnline verifies each online tool is
// independent — supplying only one credential registers only one
// extra tool.
func TestEngine_Tools_PartialOnline(t *testing.T) {
	stub := newStubModel("bash", `{}`, "")
	client, _ := chat.NewClient(stub)
	eng, err := New(Config{
		ChatClient: client,
		Online:     OnlineConfig{JinaAPIKey: "k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(eng.Tools()) != 7 {
		t.Fatalf("tool count = %d, want 7 (offline + jina only)", len(eng.Tools()))
	}
}

func toolNames(tools []chat.Tool) map[string]bool {
	out := make(map[string]bool, len(tools))
	for _, tl := range tools {
		out[tl.Definition().Name] = true
	}
	return out
}

// ------------------------------------------------------------------
// Test helpers
// ------------------------------------------------------------------

type startCall struct {
	callID    string
	toolName  string
	arguments string
}

type endCall struct {
	callID   string
	toolName string
	output   string
	err      error
}

// recordingObserver collects every Start/End/Delta the engine fires
// so the test can assert on counts, ordering, and field values. Safe
// for concurrent use — parallel tool calls would race the inner
// slices without the mutex.
type recordingObserver struct {
	mu        sync.Mutex
	startList []startCall
	endList   []endCall
	deltaList []string
}

func (r *recordingObserver) OnToolCallStart(callID, toolName, arguments string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.startList = append(r.startList, startCall{callID, toolName, arguments})
}

func (r *recordingObserver) OnToolCallEnd(callID, toolName, output string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.endList = append(r.endList, endCall{callID, toolName, output, err})
}

func (r *recordingObserver) OnMessageDelta(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deltaList = append(r.deltaList, text)
}

func (r *recordingObserver) starts() []startCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]startCall, len(r.startList))
	copy(out, r.startList)
	return out
}

func (r *recordingObserver) ends() []endCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]endCall, len(r.endList))
	copy(out, r.endList)
	return out
}

func (r *recordingObserver) deltas() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.deltaList))
	copy(out, r.deltaList)
	return out
}
