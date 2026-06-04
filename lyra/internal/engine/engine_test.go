package engine

import (
	"context"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
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

	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatalf("engine.New: %v", err)
	}

	rec := &recordingObserver{}
	out, err := eng.RunChat(context.Background(), RunChatRequest{
		Message:  "say lyra via bash",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}

	if out.Reply != "I ran echo and got lyra." {
		t.Errorf("reply mismatch: got %q", out.Reply)
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
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go"})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if out.Reply != "done" {
		t.Errorf("reply = %q, want %q", out.Reply, "done")
	}
}

// TestEngine_RunChat_RecoversFromUnknownTool proves lyra's chat action
// opts into FeedbackOnUnknownTool: when the model calls a tool that
// isn't registered, the loop feeds the error (+ real tool list) back
// and the model recovers on the next round instead of the turn
// aborting. Exercises the ActionConfig.ToolLoop → ProcessContext →
// chat tool-middleware wiring end-to-end. Without the opt-in this
// RunChat would return a "tool not registered" error.
func TestEngine_RunChat_RecoversFromUnknownTool(t *testing.T) {
	stub := newStubModel("frobnicate", `{}`, "recovered: used a real approach")
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go"})
	if err != nil {
		t.Fatalf("RunChat aborted on unknown tool (recovery not wired?): %v", err)
	}
	if out.Reply != "recovered: used a real approach" {
		t.Errorf("reply = %q, want the round-2 recovery text", out.Reply)
	}
}

// TestEngine_RunChat_TaskDelegation drives the `task` tool end-to-end:
// the main agent calls task, which spawns a fresh sub-agent (via
// AsChatToolFromAgent + SpawnChild), the sub-agent runs its own chat
// turn and returns an answer, and the main agent incorporates it into
// its final reply. Proves the sub-agent delegation path works without a
// real LLM. (The sub-agent declares ToolRoleSubtask — no `task` — so it
// can't recurse.)
func TestEngine_RunChat_TaskDelegation(t *testing.T) {
	stub := newDelegatingStubModel()
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "delegate this"})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	// Round 2 only fires if the task tool returned successfully — i.e.
	// the sub-agent spawned, ran, and produced an answer.
	if out.Reply != "main: subtask done" {
		t.Errorf("reply = %q, want the post-delegation answer", out.Reply)
	}
}

// TestEngine_RunChat_ToolsRunInCwd proves the per-run working directory
// reaches the filesystem + bash tools: a turn started with Cwd=dir runs
// `ls` and must see a file that only exists in dir. Without the cwd seam
// the tools would run in the engine's default workdir (the test process
// cwd) and the file wouldn't appear.
func TestEngine_RunChat_ToolsRunInCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}
	stub := newStubModel("bash", `{"command":"ls"}`, "done")
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	rec := &recordingObserver{}
	if _, err := eng.RunChat(context.Background(), RunChatRequest{
		Message:  "list the dir",
		Cwd:      dir,
		Observer: rec,
	}); err != nil {
		t.Fatalf("RunChat: %v", err)
	}

	ends := rec.ends()
	if len(ends) != 1 {
		t.Fatalf("tool end count = %d, want 1", len(ends))
	}
	if !strings.Contains(ends[0].output, "sentinel.txt") {
		t.Errorf("bash `ls` output %q does not list the file in Cwd %q — tools didn't run in the per-run cwd", ends[0].output, dir)
	}
}

// TestEngine_RunChat_SubtaskInheritsCwd proves the working directory reaches
// `task` sub-agents: the main turn delegates, the sub-agent's bash creates a
// marker with a RELATIVE path, and it must land in the turn's Cwd. The
// sub-agent runs on a fresh blackboard that keeps the parent's protected
// entries (SpawnChildProtectedOnly) — so it both does real work (its goal
// isn't pre-satisfied by inherited state) and inherits the cwd binding.
func TestEngine_RunChat_SubtaskInheritsCwd(t *testing.T) {
	dir := t.TempDir()
	stub := newCwdDelegatingStubModel()
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{
		Message: "delegate this",
		Cwd:     dir,
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if out.Reply != "main: subtask done" {
		t.Fatalf("reply = %q, want the post-delegation answer", out.Reply)
	}
	if _, err := os.Stat(filepath.Join(dir, "subtask_was_here.txt")); err != nil {
		t.Errorf("subtask's bash did not create the marker in Cwd %q — the sub-agent didn't run or didn't inherit the working dir: %v", dir, err)
	}
}

// TestEngine_RunChat_TokenUsageAccumulates verifies the per-turn
// usage roll-up sums across both LLM rounds (tool-call + final
// reply). ReasoningTokens come from a pointer field on chat.Usage —
// only the rounds that populate it should contribute to the total.
func TestEngine_RunChat_TokenUsageAccumulates(t *testing.T) {
	reasoning := int64(3)
	stub := newUsageStubModel(
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},
		chat.Usage{PromptTokens: 20, CompletionTokens: 7, ReasoningTokens: &reasoning},
	)
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go"})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	got := out.Usage
	want := TokenUsage{PromptTokens: 30, CompletionTokens: 12, ReasoningTokens: 3}
	if got != want {
		t.Errorf("usage = %+v, want %+v", got, want)
	}
	// Usage is read back from the process invocation ledger, and the
	// per-model breakdown rolls up to the same total under the one
	// served model.
	if len(out.UsageByModel) != 1 ||
		out.UsageByModel[0].Model != "stub-usage-model" ||
		out.UsageByModel[0].TokenUsage != want {
		t.Errorf("UsageByModel = %+v, want one entry {stub-usage-model, %+v}", out.UsageByModel, want)
	}
}

// TestEngine_RunChat_PersistsProcessSnapshot verifies the persistence
// conduit: when a ProcessStore is configured, the platform auto-snapshots
// the turn's agent process, and the persisted snapshot reflects the
// completed turn. No store → no persistence (covered by every other test
// constructing the engine without one).
func TestEngine_RunChat_PersistsProcessSnapshot(t *testing.T) {
	stub := newStreamingStubModel("done")
	client, _ := chat.NewClient(stub)
	store := core.NewInMemoryProcessStore()
	eng, err := New(context.Background(), Config{ChatClient: client, ProcessStore: store})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go"}); err != nil {
		t.Fatalf("RunChat: %v", err)
	}

	ids, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) == 0 {
		t.Fatal("expected the turn's process snapshot to be persisted")
	}
	snap, err := store.Load(context.Background(), ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != core.StatusCompleted {
		t.Errorf("snapshot status = %v, want completed", snap.Status)
	}
}

// TestEngine_RunChat_PricingFillsCost verifies the cost conduit: with a
// Pricing hook configured, each round's cost is recorded on its
// invocation and rolls up to ChatOutput.CostUSD + per-model cost. The
// rate table itself is the caller's; here a stub rate of $1/token makes
// cost == total prompt+completion tokens (30 + 12 = 42).
func TestEngine_RunChat_PricingFillsCost(t *testing.T) {
	reasoning := int64(3)
	stub := newUsageStubModel(
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},
		chat.Usage{PromptTokens: 20, CompletionTokens: 7, ReasoningTokens: &reasoning},
	)
	client, _ := chat.NewClient(stub)
	pricing := func(_ string, u *chat.Usage) float64 {
		return float64(u.PromptTokens + u.CompletionTokens)
	}
	eng, err := New(context.Background(), Config{ChatClient: client, Pricing: pricing})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go"})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if out.CostUSD != 42 {
		t.Errorf("CostUSD = %v, want 42", out.CostUSD)
	}
	if len(out.UsageByModel) != 1 || out.UsageByModel[0].CostUSD != 42 {
		t.Errorf("per-model cost = %+v, want one entry costing 42", out.UsageByModel)
	}
}

// TestEngine_RunChat_StopsOnBudget verifies the per-turn token
// ceiling halts the tool loop at a round boundary — before the next
// LLM call — and reports the partial result with StoppedOnBudget set.
// Round 1 (tool call) spends 15 tokens; with MaxBudget=10 the loop
// must stop there and never run round 2.
func TestEngine_RunChat_StopsOnBudget(t *testing.T) {
	stub := newUsageStubModel(
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},  // round 1 → total 15
		chat.Usage{PromptTokens: 99, CompletionTokens: 99}, // round 2 → must NOT run
	)
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go", MaxBudget: 10})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if !out.StoppedOnBudget {
		t.Error("expected StoppedOnBudget=true after exceeding MaxBudget")
	}
	if got := out.Usage.total(); got != 15 {
		t.Errorf("usage total = %d, want 15 (round 2 must not run)", got)
	}
}

// TestEngine_RunChat_StopsOnCostBudget verifies the dollar ceiling
// (MaxCostUSD) halts the loop the same way the token one does. With a
// $1/token stub rate, round 1 costs $15; MaxCostUSD=10 must stop there
// and never run round 2.
func TestEngine_RunChat_StopsOnCostBudget(t *testing.T) {
	stub := newUsageStubModel(
		chat.Usage{PromptTokens: 10, CompletionTokens: 5},  // round 1 → $15
		chat.Usage{PromptTokens: 99, CompletionTokens: 99}, // round 2 → must NOT run
	)
	client, _ := chat.NewClient(stub)
	pricing := func(_ string, u *chat.Usage) float64 {
		return float64(u.PromptTokens + u.CompletionTokens)
	}
	eng, err := New(context.Background(), Config{ChatClient: client, Pricing: pricing})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go", MaxCostUSD: 10})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if !out.StoppedOnBudget {
		t.Error("expected StoppedOnBudget=true after exceeding MaxCostUSD")
	}
	if out.CostUSD != 15 {
		t.Errorf("CostUSD = %v, want 15 (round 2 must not run)", out.CostUSD)
	}
}

// TestEngine_RunChat_StreamingDeltas verifies the engine forwards
// every chunk the model emits to OnMessageDelta — i.e. text is
// streamed, not buffered. The returned reply is the concatenation
// of all chunks.
func TestEngine_RunChat_StreamingDeltas(t *testing.T) {
	stub := newStreamingStubModel("Hello, ", "world!", " (lyra)")
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	rec := &recordingObserver{}
	out, err := eng.RunChat(context.Background(), RunChatRequest{
		Message:  "go",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if out.Reply != "Hello, world! (lyra)" {
		t.Errorf("reply = %q, want %q", out.Reply, "Hello, world! (lyra)")
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
	eng, err := New(context.Background(), Config{ChatClient: client})
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
// the use case for the sqlite MessageStore + cross-process
// session resume. Two engines built on the same store + same
// SessionID must see history accumulate across instances.
func TestEngine_RunChat_PersistentMemoryStoreRoundTrip(t *testing.T) {
	shared := memoryNewInMemoryStore() // built-in store; durability proxy
	stub1 := newHistoryAwareStub()
	cli1, _ := chat.NewClient(stub1)
	eng1, _ := New(context.Background(), Config{ChatClient: cli1, MemoryStore: shared})

	const sessionID = "shared-sess"
	if _, err := eng1.RunChat(context.Background(), RunChatRequest{
		SessionID: sessionID, Message: "first",
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate process restart: brand-new engine, same store.
	stub2 := newHistoryAwareStub()
	cli2, _ := chat.NewClient(stub2)
	eng2, _ := New(context.Background(), Config{ChatClient: cli2, MemoryStore: shared})

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
	eng, err := New(context.Background(), Config{ChatClient: client})
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
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	tools := eng.Tools()
	// 6 offline coding tools + the always-present `task` delegation tool.
	if len(tools) != 7 {
		t.Fatalf("tool count = %d, want 7 (6 offline + task)", len(tools))
	}

	names := toolNames(tools)
	for _, want := range []string{"read", "write", "edit", "glob", "grep", "bash", "task"} {
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
	eng, err := New(context.Background(), Config{
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
	if len(tools) != 10 {
		t.Fatalf("tool count = %d, want 10 (6 offline + 3 online + task)", len(tools))
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
	eng, err := New(context.Background(), Config{
		ChatClient: client,
		Online:     OnlineConfig{JinaAPIKey: "k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(eng.Tools()) != 8 {
		t.Fatalf("tool count = %d, want 8 (6 offline + jina + task)", len(eng.Tools()))
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
	planList  []string
}

func (r *recordingObserver) ApproveToolCall(_ context.Context, _, _, _ string) ToolApprovalVerdict {
	return ToolApprovalVerdict{} // auto-run; tests don't exercise the approval gate
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

// OnReasoningDelta is a no-op for the current tests — reasoning
// streams aren't asserted at the engine level. Lyra-level tests
// in chat/impl_test.go cover the propagation path.
func (r *recordingObserver) OnReasoningDelta(_ string) {}

// OnPlanGenerated records the drafted plan so plan-mode tests can assert
// it fired before the process parked on approval.
func (r *recordingObserver) OnPlanGenerated(plan string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.planList = append(r.planList, plan)
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

// namedUsageStub reports a configurable served-model name (and 1/1 usage) in
// a single round — used to detect which client a turn actually ran against.
type namedUsageStub struct {
	model    string
	defaults *chat.Options
}

func newNamedStub(model string) *namedUsageStub {
	opts, _ := chat.NewOptions(model)
	return &namedUsageStub{model: model, defaults: opts}
}

func (m *namedUsageStub) DefaultOptions() chat.Options { return *m.defaults }
func (m *namedUsageStub) Metadata() chat.ModelMetadata { return chat.ModelMetadata{Provider: "stub"} }

func (m *namedUsageStub) Call(_ context.Context, _ *chat.Request) (*chat.Response, error) {
	u := chat.Usage{PromptTokens: 1, CompletionTokens: 1}
	resp, err := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage("ok"),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{Usage: &u},
	)
	if resp != nil && resp.Metadata != nil {
		resp.Metadata.Model = m.model
	}
	return resp, err
}

func (m *namedUsageStub) Stream(ctx context.Context, req *chat.Request) iter.Seq2[*chat.Response, error] {
	resp, err := m.Call(ctx, req)
	return func(yield func(*chat.Response, error) bool) { yield(resp, err) }
}

// TestEngine_RunChat_PerRunClientOverride verifies RunChatRequest.ChatClient
// actually drives the turn's LLM call (via the ChatClientProvider seam),
// not the platform's default client.
func TestEngine_RunChat_PerRunClientOverride(t *testing.T) {
	defClient, _ := chat.NewClient(newNamedStub("default-model"))
	ovrClient, _ := chat.NewClient(newNamedStub("override-model"))
	eng, err := New(context.Background(), Config{ChatClient: defClient})
	if err != nil {
		t.Fatal(err)
	}
	out, err := eng.RunChat(context.Background(), RunChatRequest{Message: "go", ChatClient: ovrClient})
	if err != nil {
		t.Fatalf("RunChat: %v", err)
	}
	if len(out.UsageByModel) != 1 || out.UsageByModel[0].Model != "override-model" {
		t.Fatalf("UsageByModel = %+v, want served model override-model", out.UsageByModel)
	}
}
