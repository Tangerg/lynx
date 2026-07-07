package kernel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/core/model/chat"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
)

// TestEngine_RunChat_ToolCallObserved drives the engine with a stub
// model that asks for a `shell` tool call (echo lyra), then returns a
// final text mentioning the captured output. The observer must see
// one OnToolCallStart / OnToolCallEnd pair; the returned reply must
// be the stub's FinalText.
//
// This is the M2-readiness gate: it proves the chain
// engine.StartTurn → lynx Platform → tool loop → tool decorator
// → observedTool → toolObserver is wired end-to-end without any
// real LLM in the loop.
func TestEngine_RunChat_ToolCallObserved(t *testing.T) {
	stub := newStubModel("shell", `{"command":"echo lyra"}`, "I ran echo and got lyra.")
	client, err := chat.NewClient(stub)
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	rec := &recordingObserver{}
	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{
		Message:  "say lyra via shell",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	if out.Reply != "I ran echo and got lyra." {
		t.Errorf("reply mismatch: got %q", out.Reply)
	}

	starts := rec.starts()
	ends := rec.ends()

	if len(starts) != 1 {
		t.Fatalf("OnToolCallStart count = %d, want 1; got %#v", len(starts), starts)
	}
	if starts[0].toolName != "shell" {
		t.Errorf("start tool name = %q, want shell", starts[0].toolName)
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
	stub := newStubModel("shell", `{"command":"echo lyra"}`, "done")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
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
// The turn would return a "tool not registered" error.
func TestEngine_RunChat_RecoversFromUnknownTool(t *testing.T) {
	stub := newStubModel("frobnicate", `{}`, "recovered: used a real approach")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("RunTurn aborted on unknown tool (recovery not wired?): %v", err)
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
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{Message: "delegate this"})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	// Round 2 only fires if the task tool returned successfully — i.e.
	// the sub-agent spawned, ran, and produced an answer.
	if out.Reply != "main: subtask done" {
		t.Errorf("reply = %q, want the post-delegation answer", out.Reply)
	}
}

// TestEngine_RunChat_ToolsRunInCwd proves the per-run working directory
// reaches the filesystem + shell tools: a turn started with Cwd=dir runs
// `ls` and must see a file that only exists in dir. Without the cwd seam
// the tools would run in the engine's default workdir (the test process
// cwd) and the file wouldn't appear.
func TestEngine_RunChat_ToolsRunInCwd(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "sentinel.txt"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}
	stub := newStubModel("shell", `{"command":"ls"}`, "done")
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	rec := &recordingObserver{}
	if _, err := eng.runTurnSync(context.Background(), RunTurnRequest{
		Message:  "list the dir",
		Cwd:      dir,
		Observer: rec,
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	ends := rec.ends()
	if len(ends) != 1 {
		t.Fatalf("tool end count = %d, want 1", len(ends))
	}
	if !strings.Contains(ends[0].output, "sentinel.txt") {
		t.Errorf("shell `ls` output %q does not list the file in Cwd %q — tools didn't run in the per-run cwd", ends[0].output, dir)
	}
}

// TestEngine_RunChat_SubtaskInheritsCwd proves the working directory reaches
// `task` sub-agents: the main turn delegates, the sub-agent's shell creates a
// marker with a RELATIVE path, and it must land in the turn's Cwd. The
// sub-agent runs on a fresh blackboard that keeps the parent's protected
// entries (SpawnChildProtectedOnly) — so it both does real work (its goal
// isn't pre-satisfied by inherited state) and inherits the cwd binding.
func TestEngine_RunChat_SubtaskInheritsCwd(t *testing.T) {
	dir := t.TempDir()
	stub := newCwdDelegatingStubModel()
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{
		Message: "delegate this",
		Cwd:     dir,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if out.Reply != "main: subtask done" {
		t.Fatalf("reply = %q, want the post-delegation answer", out.Reply)
	}
	if _, err := os.Stat(filepath.Join(dir, "subtask_was_here.txt")); err != nil {
		t.Errorf("subtask's shell did not create the marker in Cwd %q — the sub-agent didn't run or didn't inherit the working dir: %v", dir, err)
	}
}

// TestEngine_RunChat_SubtaskKeepsHistoryAcrossRounds is the regression guard
// for subtask chat-history continuity. A subtask runs its own multi-round
// tool loop with no externally-supplied session; the tool loop strips the
// original prompt between rounds, so round 2 only sees it if the child's
// history middleware reconstructs it — which requires the runtime to stamp a
// conversation id (here the child's process id) onto every request. The
// subtask is told a secret on round 1 and must echo it on round 2; if the
// per-process keying regresses, the subtask reports subtaskContextLost and
// the main turn surfaces it.
func TestEngine_RunChat_SubtaskKeepsHistoryAcrossRounds(t *testing.T) {
	stub := newSubtaskMemoryStub()
	client, _ := chat.NewClient(stub)
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{
		Message: "delegate this",
		Cwd:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if strings.Contains(out.Reply, subtaskContextLost) {
		t.Fatalf("subtask lost its round-1 context across tool rounds — per-process chat-history keying regressed; reply = %q", out.Reply)
	}
	if !strings.Contains(out.Reply, subtaskSecret) {
		t.Errorf("reply = %q, want it to carry the subtask's secret %q (proof round-2 saw round-1's prompt)", out.Reply, subtaskSecret)
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
	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{
		Message:  "go",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
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

func TestEngine_RunChat_PassesOptions(t *testing.T) {
	stub := newStreamingStubModel("ok")
	client, _ := chat.NewClient(stub)
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}
	temp := 0.7
	maxTokens := int64(256)

	if _, err := eng.runTurnSync(context.Background(), RunTurnRequest{
		Message: "go",
		Options: &chat.Options{
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			Stop:        []string{"END"},
		},
	}); err != nil {
		t.Fatalf("RunTurn: %v", err)
	}

	stub.mu.Lock()
	got := stub.lastOptions
	stub.mu.Unlock()
	if got == nil {
		t.Fatal("model saw nil options")
	}
	if got.Model != "stub-model-streaming" {
		t.Fatalf("Model = %q, want default stub-model-streaming", got.Model)
	}
	if got.Temperature == nil || *got.Temperature != 0.7 {
		t.Fatalf("Temperature = %v, want 0.7", got.Temperature)
	}
	if got.MaxTokens == nil || *got.MaxTokens != 256 {
		t.Fatalf("MaxTokens = %v, want 256", got.MaxTokens)
	}
	if len(got.Stop) != 1 || got.Stop[0] != "END" {
		t.Fatalf("Stop = %v, want END", got.Stop)
	}
}

func TestEngine_RestoreChat_PreservesOptionsFromSnapshot(t *testing.T) {
	stub := newOptionToolStub()
	client, _ := chat.NewClient(stub)
	store := newJSONProcessStore()
	built, err := toolset.Build(context.Background(), toolset.BuildConfig{})
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	eng, err := New(context.Background(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Tools:        built.Tools,
		MCP:          built.MCP,
		Closers:      built.Closers,
		ProcessStore: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	temp := 0.42
	maxTokens := int64(321)
	observer := &hitlApprovalObserver{}

	proc := eng.StartTurn(context.Background(), RunTurnRequest{
		Message:  "echo lyra",
		Observer: observer,
		Options: &chat.Options{
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			Stop:        []string{"END"},
		},
	})
	if err := <-proc.Done(); err != nil {
		t.Fatalf("initial StartTurn: %v", err)
	}
	if proc.Status() != core.StatusWaiting {
		t.Fatalf("initial status = %s, want waiting", proc.Status())
	}

	eng2, err := New(context.Background(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Tools:        built.Tools,
		MCP:          built.MCP,
		Closers:      built.Closers,
		ProcessStore: store,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer eng2.Close()

	restored, err := eng2.RestoreTurn(context.Background(), proc.ID(), RestoreTurnRequest{
		Observer: observer,
	})
	if err != nil {
		t.Fatalf("RestoreTurn: %v", err)
	}
	if restored.Status() != core.StatusWaiting {
		t.Fatalf("restored status = %s, want waiting", restored.Status())
	}
	done, err := restored.Resume(context.Background(), interrupts.Resolution{Approved: true})
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("resumed run: %v", err)
	}
	out, err := restored.Output()
	if err != nil {
		t.Fatalf("Output: %v", err)
	}
	if out.Reply != "restored ok" {
		t.Fatalf("reply = %q, want restored ok", out.Reply)
	}

	got := stub.lastCapturedOptions()
	if got == nil {
		t.Fatal("model saw nil options after restore")
	}
	if got.Model != "stub-options-restore" {
		t.Fatalf("Model = %q, want stub-options-restore", got.Model)
	}
	if got.Temperature == nil || *got.Temperature != temp {
		t.Fatalf("Temperature = %v, want %v", got.Temperature, temp)
	}
	if got.MaxTokens == nil || *got.MaxTokens != maxTokens {
		t.Fatalf("MaxTokens = %v, want %v", got.MaxTokens, maxTokens)
	}
	if len(got.Stop) != 1 || got.Stop[0] != "END" {
		t.Fatalf("Stop = %v, want END", got.Stop)
	}
}

// TestEngine_RunChat_PerRunClientOverride verifies RunTurnRequest.ChatClient
// actually drives the turn's LLM call (via the ChatClientProvider seam),
// not the platform's default client.
func TestEngine_RunChat_PerRunClientOverride(t *testing.T) {
	defClient, _ := chat.NewClient(newNamedStub("default-model"))
	ovrClient, _ := chat.NewClient(newNamedStub("override-model"))
	eng, err := New(context.Background(), Config{ChatClient: defClient})
	if err != nil {
		t.Fatal(err)
	}
	out, err := eng.runTurnSync(context.Background(), RunTurnRequest{Message: "go", ChatClient: ovrClient})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if len(out.UsageByModel) != 1 || out.UsageByModel[0].Model != "override-model" {
		t.Fatalf("UsageByModel = %+v, want served model override-model", out.UsageByModel)
	}
}
