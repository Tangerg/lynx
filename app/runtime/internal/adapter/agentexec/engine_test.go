package agentexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/chatclient"
	history "github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/core/media"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// TestEngine_RunChat_ToolCallObserved drives the engine with a stub
// model that asks for a `shell` tool call (echo lyra), then returns a
// final text mentioning the captured output. The observer must see
// one OnToolCallStart / OnToolCallEnd pair; the returned reply must
// be the stub's FinalText.
//
// This is the M2-readiness gate: it proves the chain
// engine.StartTurn → lynx Engine → tool loop → tool decorator
// → observedTool → executionObserver is wired end-to-end without any
// real LLM in the loop.
func TestEngine_RunChat_ToolCallObserved(t *testing.T) {
	stub := newStubModel("shell", `{"command":"echo lyra","description":"Print lyra"}`, "I ran echo and got lyra.")
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}

	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	rec := &recordingObserver{}
	out, err := eng.runTurnSync(context.Background(), TurnRequest{
		Message:  "say lyra via shell",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
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
	if !strings.HasPrefix(starts[0].callID, "model:") {
		t.Errorf("managed call id = %q, want model-owned stable identity", starts[0].callID)
	}
	if starts[0].sourceCallID != "call_1" {
		t.Errorf("source call id = %q, want provider identity call_1", starts[0].sourceCallID)
	}
}

// TestEngine_RunChat_NoObserver verifies the nil-observer path: the
// engine still drives the tool loop, just without firing any
// notifications.
func TestEngine_RunChat_NoObserver(t *testing.T) {
	stub := newStubModel("shell", `{"command":"echo lyra","description":"Print lyra"}`, "done")
	client, _ := chatclient.New(stub, chatclient.Config{Defaults: *stub.defaults})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if out.Reply != "done" {
		t.Errorf("reply = %q, want %q", out.Reply, "done")
	}
}

func TestEngine_RunChat_MediaOnlyInput(t *testing.T) {
	stub := newStreamingStubModel("described image")
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	image, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("media.NewBytes: %v", err)
	}
	out, err := eng.runTurnSync(context.Background(), TurnRequest{Media: []*media.Media{image}})
	if err != nil {
		t.Fatalf("runTurnSync media-only input: %v", err)
	}
	if out.Reply != "described image" {
		t.Fatalf("reply = %q, want described image", out.Reply)
	}
	messages := stub.capturedMessages()
	if len(messages) != 2 || len(messages[1].Parts) != 1 || messages[1].Parts[0].Kind != chat.PartMedia {
		t.Fatalf("model messages = %+v, want system plus media-only user message", messages)
	}
}

func TestEngine_RunChat_TextAndMediaInput(t *testing.T) {
	stub := newStreamingStubModel("described image")
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	image, err := media.NewBytes("image/png", []byte{1, 2, 3})
	if err != nil {
		t.Fatalf("media.NewBytes: %v", err)
	}
	if _, err := eng.runTurnSync(context.Background(), TurnRequest{
		Message: "describe this",
		Media:   []*media.Media{image},
	}); err != nil {
		t.Fatalf("runTurnSync text and media input: %v", err)
	}
	messages := stub.capturedMessages()
	if len(messages) != 2 || len(messages[1].Parts) != 2 {
		t.Fatalf("model messages = %+v, want system plus text-and-media user message", messages)
	}
	if messages[1].Parts[0].Kind != chat.PartText || messages[1].Parts[0].Text != "describe this" {
		t.Fatalf("first user part = %+v, want text", messages[1].Parts[0])
	}
	if messages[1].Parts[1].Kind != chat.PartMedia {
		t.Fatalf("second user part = %+v, want media", messages[1].Parts[1])
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
	client, _ := chatclient.New(stub, chatclient.Config{Defaults: *stub.defaults})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("runTurnSync aborted on unknown tool (recovery not wired?): %v", err)
	}
	if out.Reply != "recovered: used a real approach" {
		t.Errorf("reply = %q, want the round-2 recovery text", out.Reply)
	}
}

// TestEngine_RunChat_TaskDelegation drives `delegate_task` end-to-end:
// the main agent calls task, which spawns a fresh sub-agent (via
// NewAgentTool + RunChild), the sub-agent runs its own chat
// turn and returns an answer, and the main agent incorporates it into
// its final reply. Proves the sub-agent delegation path works without a
// real LLM.
func TestEngine_RunChat_TaskDelegation(t *testing.T) {
	stub := newDelegatingStubModel()
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "delegate this"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	// Round 2 only fires if the task tool returned successfully — i.e.
	// the sub-agent spawned, ran, and produced an answer.
	if out.Reply != "main: subtask done" {
		t.Errorf("reply = %q, want the post-delegation answer", out.Reply)
	}
}

func TestEngine_ProjectsDelegatedChildCompletionWithExactSubtreeUsage(t *testing.T) {
	stub := newDelegatingAccountingStub("stub-delegating", chat.Usage{
		InputTokens:  11,
		OutputTokens: 3,
	})
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	observer := &recordingObserver{}
	admitted := make(chan ChildProcess, 1)
	out, err := eng.runTurnSync(t.Context(), TurnRequest{
		Message:  "delegate this",
		Observer: observer,
		AdmitChild: func(_ context.Context, child ChildProcess) error {
			admitted <- child
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}

	child := <-admitted
	if child.ID == "" || child.ParentID == "" || child.SpawnCallID == "" || child.StartedAt.IsZero() {
		t.Fatalf("admitted child has incomplete identity: %+v", child)
	}
	completions := observer.childCompletions()
	if len(completions) != 1 {
		t.Fatalf("child completions = %+v, want exactly one", completions)
	}
	completion := completions[0]
	if completion.Process != child {
		t.Fatalf("completed child = %+v, admitted %+v", completion.Process, child)
	}
	if completion.Status != core.StatusCompleted ||
		completion.StopReason != agent.InteractionStopNone ||
		completion.Err != nil ||
		completion.CompletedAt.Before(completion.Process.StartedAt) {
		t.Fatalf("child completion = %+v", completion)
	}
	if completion.Usage.PromptTokens != 11 ||
		completion.Usage.CompletionTokens != 3 ||
		completion.Steps != 1 ||
		len(completion.UsageByModel) != 1 ||
		completion.UsageByModel[0].Model != "stub-delegating" ||
		completion.UsageByModel[0].Calls != 1 {
		t.Fatalf("child subtree usage = %+v / %+v", completion.Usage, completion.UsageByModel)
	}

	if out.Usage.PromptTokens != 33 ||
		out.Usage.CompletionTokens != 9 ||
		out.Steps != 3 ||
		len(out.UsageByModel) != 1 ||
		out.UsageByModel[0].Calls != 3 {
		t.Fatalf("root subtree usage = %+v / %+v", out.Usage, out.UsageByModel)
	}
	var childProgress []usageObservation
	for _, observation := range observer.usages() {
		if observation.process == child.ProcessRef {
			childProgress = append(childProgress, observation)
		}
	}
	if len(childProgress) != 1 ||
		childProgress[0].usage != completion.Usage ||
		childProgress[0].steps != completion.Steps ||
		childProgress[0].contextTokens != 11 {
		t.Fatalf("child progress = %+v, completion usage %+v", childProgress, completion.Usage)
	}

	order := observer.eventOrder()
	childEnd := slices.Index(order, "child-end:"+child.ID)
	parentToolEnd := slices.Index(order, "tool-end:"+child.ParentID+":delegate_task")
	if childEnd < 0 || parentToolEnd < 0 || childEnd >= parentToolEnd {
		t.Fatalf("event order = %v, want child terminal before parent delegation result", order)
	}
}

func TestEngine_ProjectsNestedDelegationInPostorder(t *testing.T) {
	stub := newNestedDelegatingStub()
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	observer := &recordingObserver{}
	admitted := make(chan ChildProcess, 2)
	out, err := eng.runTurnSync(t.Context(), TurnRequest{
		Message:  "nested root",
		Observer: observer,
		AdmitChild: func(_ context.Context, child ChildProcess) error {
			admitted <- child
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	first := <-admitted
	second := <-admitted
	if second.ParentID != first.ID || first.ParentID == "" {
		t.Fatalf("admission lineage root → child → grandchild = %+v → %+v", first, second)
	}

	completions := observer.childCompletions()
	if len(completions) != 2 {
		t.Fatalf("child completions = %+v, want grandchild and child", completions)
	}
	grandchild, child := completions[0], completions[1]
	if grandchild.Process != second ||
		grandchild.Steps != 1 ||
		grandchild.Usage.PromptTokens != 7 ||
		len(grandchild.UsageByModel) != 1 ||
		grandchild.UsageByModel[0].Calls != 1 {
		t.Fatalf("grandchild completion = %+v", grandchild)
	}
	if child.Process != first ||
		child.Steps != 3 ||
		child.Usage.PromptTokens != 21 ||
		len(child.UsageByModel) != 1 ||
		child.UsageByModel[0].Calls != 3 {
		t.Fatalf("child subtree completion = %+v", child)
	}
	if out.Reply != "root: result" ||
		out.Steps != 5 ||
		out.Usage.PromptTokens != 35 ||
		len(out.UsageByModel) != 1 ||
		out.UsageByModel[0].Calls != 5 ||
		stub.Calls() != 5 {
		t.Fatalf("root subtree output = %+v, provider calls = %d", out, stub.Calls())
	}

	order := observer.eventOrder()
	grandchildEnd := slices.Index(order, "child-end:"+second.ID)
	childToolEnd := slices.Index(order, "tool-end:"+first.ID+":delegate_task")
	childEnd := slices.Index(order, "child-end:"+first.ID)
	rootToolEnd := slices.Index(order, "tool-end:"+first.ParentID+":delegate_task")
	if grandchildEnd < 0 ||
		childToolEnd < 0 ||
		childEnd < 0 ||
		rootToolEnd < 0 ||
		grandchildEnd >= childToolEnd ||
		childToolEnd >= childEnd ||
		childEnd >= rootToolEnd {
		t.Fatalf("nested event order = %v", order)
	}
}

func TestEngine_ProjectsConcurrentSiblingsWithoutAccountingContamination(t *testing.T) {
	stub := newSiblingDelegatingStub()
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatalf("chat client: %v", err)
	}
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	observer := &recordingObserver{}
	admitted := make(chan ChildProcess, 2)
	out, err := eng.runTurnSync(t.Context(), TurnRequest{
		Message:  "run siblings",
		Observer: observer,
		AdmitChild: func(_ context.Context, child ChildProcess) error {
			admitted <- child
			return nil
		},
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	children := []ChildProcess{<-admitted, <-admitted}
	if children[0].ParentID == "" ||
		children[1].ParentID != children[0].ParentID ||
		children[0].ID == children[1].ID ||
		children[0].SpawnCallID == children[1].SpawnCallID {
		t.Fatalf("sibling admissions = %+v", children)
	}

	completions := observer.childCompletions()
	if len(completions) != 2 {
		t.Fatalf("sibling completions = %+v", completions)
	}
	for _, completion := range completions {
		if completion.Process.ParentID != children[0].ParentID ||
			completion.Steps != 1 ||
			completion.Usage.PromptTokens != 5 ||
			completion.Usage.CompletionTokens != 1 ||
			len(completion.UsageByModel) != 1 ||
			completion.UsageByModel[0].Calls != 1 {
			t.Fatalf("sibling completion contains cross-process usage: %+v", completion)
		}
	}
	if out.Reply != "root: siblings done" ||
		out.Steps != 4 ||
		out.Usage.PromptTokens != 20 ||
		out.Usage.CompletionTokens != 4 ||
		len(out.UsageByModel) != 1 ||
		out.UsageByModel[0].Calls != 4 ||
		stub.Calls() != 4 {
		t.Fatalf("root sibling aggregate = %+v, provider calls = %d", out, stub.Calls())
	}

	order := observer.eventOrder()
	canonicalCallBySource := make(map[string]string, len(children))
	for _, start := range observer.starts() {
		if start.process.ID == children[0].ParentID && start.toolName == "delegate_task" {
			canonicalCallBySource[start.sourceCallID] = start.callID
		}
	}
	endOrdinalByCall := make(map[string]int, len(children))
	taskEndCount := 0
	for _, end := range observer.ends() {
		if end.process.ID == children[0].ParentID && end.toolName == "delegate_task" {
			endOrdinalByCall[end.callID] = taskEndCount
			taskEndCount++
		}
	}
	for _, child := range children {
		childEnd := slices.Index(order, "child-end:"+child.ID)
		callID := canonicalCallBySource[child.SpawnCallID]
		endOrdinal, ok := endOrdinalByCall[callID]
		parentToolEnd := nthIndex(order, "tool-end:"+child.ParentID+":delegate_task", endOrdinal)
		if callID == "" || !ok || childEnd < 0 || parentToolEnd < 0 || childEnd >= parentToolEnd {
			t.Fatalf(
				"sibling event order = %v, child %q (%s → %s) did not end before its delegation call",
				order,
				child.ID,
				child.SpawnCallID,
				callID,
			)
		}
	}
}

func nthIndex(values []string, target string, ordinal int) int {
	if ordinal < 0 {
		return -1
	}
	for index, value := range values {
		if value != target {
			continue
		}
		if ordinal == 0 {
			return index
		}
		ordinal--
	}
	return -1
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
	stub := newStubModel("shell", `{"command":"ls","description":"List workspace files"}`, "done")
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	rec := &recordingObserver{}
	if _, err := eng.runTurnSync(context.Background(), TurnRequest{
		Message:  "list the dir",
		Cwd:      dir,
		Observer: rec,
	}); err != nil {
		t.Fatalf("runTurnSync: %v", err)
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
// delegated Agents: the main turn delegates, the child's shell creates a
// marker with a RELATIVE path, and it must land in the turn's Cwd. The
// sub-agent runs on a clean blackboard — so its goal is not pre-satisfied by
// inherited planner state — while the Application-owned context carries the cwd.
func TestEngine_RunChat_SubtaskInheritsCwd(t *testing.T) {
	dir := t.TempDir()
	stub := newCwdDelegatingStubModel()
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), TurnRequest{
		Message: "delegate this",
		Cwd:     dir,
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
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
// original prompt between rounds, so round 2 only sees it if the app-owned
// history middleware reconstructs it under the child's process ID. The
// subtask is told a secret on round 1 and must echo it on round 2; if the
// per-process keying regresses, the subtask reports subtaskContextLost and
// the main turn surfaces it.
func TestEngine_RunChat_SubtaskKeepsHistoryAcrossRounds(t *testing.T) {
	stub := newSubtaskMemoryStub()
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()

	out, err := eng.runTurnSync(context.Background(), TurnRequest{
		Message: "delegate this",
		Cwd:     t.TempDir(),
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
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
	client, _ := chatclient.New(stub, chatclient.Config{})
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	rec := &recordingObserver{}
	out, err := eng.runTurnSync(context.Background(), TurnRequest{
		Message:  "go",
		Observer: rec,
	})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
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

func TestEngine_RunChat_ModelResponseFinalIsAuthoritative(t *testing.T) {
	stub := newChoiceOrderStubModel()
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if out.Reply != "authoritative" {
		t.Fatalf("reply = %q, want tagged final response text", out.Reply)
	}
}

func TestEngine_RunChat_DirectToolResultIsFinal(t *testing.T) {
	direct, err := newDirectResultTool()
	if err != nil {
		t.Fatal(err)
	}
	stub := newDirectReturnStubModel()
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(context.Background(), Config{
		ChatClient:   client,
		ToolResolver: &fixedToolResolver{tool: direct},
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "finish"})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if out.Reply != "direct result" {
		t.Fatalf("reply = %q, want direct result", out.Reply)
	}
	if stub.Calls() != 1 {
		t.Fatalf("model calls = %d, want 1 for return-direct tool", stub.Calls())
	}
}

func TestEngine_RunChat_ArtificialStopsPreservePartialText(t *testing.T) {
	tests := []struct {
		name       string
		request    TurnRequest
		wantReason agent.InteractionStopReason
	}{
		{
			name:       "budget",
			request:    TurnRequest{Message: "go", Limits: execution.RunLimits{MaxTotalTokens: 10}},
			wantReason: agent.InteractionStopBudget,
		},
		{
			name:       "steps",
			request:    TurnRequest{Message: "go", Limits: execution.RunLimits{MaxSteps: 1}},
			wantReason: agent.InteractionStopModelCalls,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stub := newPartialStopStubModel()
			client, err := chatclient.New(stub, chatclient.Config{})
			if err != nil {
				t.Fatal(err)
			}
			eng := mustEngineWith(t, client, toolset.BuildConfig{})
			defer eng.Close()

			out, err := eng.runTurnSync(context.Background(), test.request)
			if err != nil {
				t.Fatalf("runTurnSync: %v", err)
			}
			if out.Reply != "partial answer" {
				t.Fatalf("reply = %q, want partial answer", out.Reply)
			}
			if out.StopReason != test.wantReason {
				t.Fatalf("StopReason = %q, want %q", out.StopReason, test.wantReason)
			}
			if stub.Calls() != 1 {
				t.Fatalf("model calls = %d, want 1", stub.Calls())
			}
		})
	}
}

func TestEngine_RunChat_LongToolDoesNotTripModelIdleTimeout(t *testing.T) {
	stub := newStubModel("shell", `{"command":"sleep 0.08; echo complete","description":"Wait then print complete"}`, "done")
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()
	eng.modelStreamIdleTimeout = 20 * time.Millisecond

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		t.Fatalf("long tool was killed by model idle timeout: %v", err)
	}
	if out.Reply != "done" {
		t.Fatalf("reply = %q, want done", out.Reply)
	}
}

func TestEngine_RunChat_ToolTimeoutIsNotModelIdleTimeout(t *testing.T) {
	stub := newStubModel("shell", `{"command":"sleep 0.08","description":"Wait briefly","timeout_ms":10}`, "recovered")
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	eng := mustEngineWith(t, client, toolset.BuildConfig{})
	defer eng.Close()
	eng.modelStreamIdleTimeout = 100 * time.Millisecond

	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go"})
	if err != nil {
		if errors.Is(err, errModelStreamIdleTimeout) || strings.Contains(err.Error(), "model stream idle") {
			t.Fatalf("tool timeout misreported as model idle: %v", err)
		}
		t.Fatalf("runTurnSync: %v", err)
	}
	if out.Reply != "recovered" {
		t.Fatalf("reply = %q, want recovered", out.Reply)
	}
}

func TestEngine_StartTurn_PropagatesSteeringGuardrailConstructionError(t *testing.T) {
	stub := newStreamingStubModel("unused")
	client, err := chatclient.New(stub, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("guardrail construction failed")
	eng.chatMiddlewareBuilder = func(history.Store, func(context.Context) []chat.Message) (*core.ChatMiddleware, error) {
		return nil, sentinel
	}

	process, err := eng.StartTurn(context.Background(), TurnRequest{
		SessionID: "session",
		Message:   "go",
		Steer:     func() []chat.Message { return nil },
	})
	if process != nil {
		t.Fatal("StartTurn returned a process after guardrail construction failed")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("StartTurn error = %v, want %v", err, sentinel)
	}
}

func TestEngine_RunChat_PassesOptions(t *testing.T) {
	stub := newStreamingStubModel("ok")
	client, _ := chatclient.New(stub, chatclient.Config{Defaults: *stub.defaults})
	eng, err := New(context.Background(), Config{ChatClient: client})
	if err != nil {
		t.Fatal(err)
	}
	temp := 0.7
	maxTokens := int64(256)

	if _, err := eng.runTurnSync(context.Background(), TurnRequest{
		Message: "go",
		Options: &chat.Options{
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			Stop:        []string{"END"},
		},
	}); err != nil {
		t.Fatalf("runTurnSync: %v", err)
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
	client, _ := chatclient.New(stub, chatclient.Config{Defaults: *stub.defaults})
	store := newMemoryCheckpointStore()
	built, err := toolset.Build(context.Background(), testToolsetBuildConfig(t, toolset.BuildConfig{}))
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupBuiltTools(t, built)
	eng, err := New(context.Background(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Checkpoints:  store,
		BuildID:      testBuildID,
		Pricing:      func(string, string, *chat.Usage) float64 { return 0.25 },
	})
	if err != nil {
		t.Fatal(err)
	}
	temp := 0.42
	maxTokens := int64(321)
	observer := &hitlApprovalObserver{}
	wantScope := execution.ExecutionScope{
		SessionID:    "session-restore",
		Cwd:          "/sandbox/restore",
		WorkspaceCwd: "/workspace/restore",
		Isolated:     true,
		GoalLeaseID:  "goal-lease-restore",
	}
	wantProvider := "selected-provider"
	wantSelection := mustTestSelection(t, wantProvider, "selected-model")
	wantLimits := execution.RunLimits{
		MaxTotalTokens: 10_000,
		MaxBudgetUSD:   10,
		MaxSteps:       4,
	}

	proc, err := eng.StartTurn(context.Background(), TurnRequest{
		SessionID:      wantScope.SessionID,
		Message:        "echo lyra",
		ModelSelection: wantSelection,
		Cwd:            wantScope.Cwd,
		WorkspaceCwd:   wantScope.WorkspaceCwd,
		Isolated:       wantScope.Isolated,
		GoalLeaseID:    wantScope.GoalLeaseID,
		Limits:         wantLimits,
		Observer:       observer,
		Options: &chat.Options{
			Temperature: &temp,
			MaxTokens:   &maxTokens,
			Stop:        []string{"END"},
		},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	initial := proc.Await()
	if initial.Err != nil {
		t.Fatalf("initial StartTurn: %v", initial.Err)
	}
	if initial.Status != core.StatusWaiting {
		t.Fatalf("initial status = %s, want waiting", initial.Status)
	}
	persistWaitingCheckpoint(t, store, proc)

	eng2, err := New(context.Background(), Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Checkpoints:  store,
		BuildID:      testBuildID,
		Pricing:      func(string, string, *chat.Usage) float64 { return 0.25 },
	})
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint, err := store.LoadCheckpoint(t.Context(), proc.ID()); err != nil {
		t.Fatalf("load checkpoint metadata: %v", err)
	} else if checkpoint.Scope != wantScope ||
		checkpoint.ModelSelection != wantSelection ||
		checkpoint.Limits != wantLimits {
		t.Fatalf(
			"checkpoint policy = scope:%+v model:%q/%q budget:%+v, want scope:%+v model:%q/%q budget:%+v",
			checkpoint.Scope,
			checkpoint.ModelSelection.Provider(),
			checkpoint.ModelSelection.Model(),
			checkpoint.Limits,
			wantScope,
			wantSelection.Provider(),
			wantSelection.Model(),
			wantLimits,
		)
	}

	if mismatched, err := eng2.RestoreTurn(context.Background(), proc.ID(), RestoreTurnRequest{
		SessionID:      "another-session",
		ModelSelection: wantSelection,
		Cwd:            wantScope.Cwd,
		WorkspaceCwd:   wantScope.WorkspaceCwd,
		Isolated:       wantScope.Isolated,
		GoalLeaseID:    wantScope.GoalLeaseID,
		Limits:         wantLimits,
		Observer:       observer,
	}); mismatched != nil || !errors.Is(err, ErrExecutorCheckpointLost) {
		t.Fatalf("cross-session RestoreTurn = (%T, %v), want checkpoint loss", mismatched, err)
	}
	if mismatched, err := eng2.RestoreTurn(context.Background(), proc.ID(), RestoreTurnRequest{
		SessionID:      wantScope.SessionID,
		ModelSelection: mustTestSelection(t, "another-provider", wantSelection.Model()),
		Cwd:            wantScope.Cwd,
		WorkspaceCwd:   wantScope.WorkspaceCwd,
		Isolated:       wantScope.Isolated,
		GoalLeaseID:    wantScope.GoalLeaseID,
		Limits:         wantLimits,
		Observer:       observer,
	}); mismatched != nil || !errors.Is(err, ErrExecutorCheckpointLost) {
		t.Fatalf("cross-provider RestoreTurn = (%T, %v), want checkpoint loss", mismatched, err)
	}
	if mismatched, err := eng2.RestoreTurn(context.Background(), proc.ID(), RestoreTurnRequest{
		SessionID:      wantScope.SessionID,
		ModelSelection: mustTestSelection(t, wantSelection.Provider(), "another-model"),
		Cwd:            wantScope.Cwd,
		WorkspaceCwd:   wantScope.WorkspaceCwd,
		Isolated:       wantScope.Isolated,
		GoalLeaseID:    wantScope.GoalLeaseID,
		Limits:         wantLimits,
		Observer:       observer,
	}); mismatched != nil || !errors.Is(err, ErrExecutorCheckpointLost) {
		t.Fatalf("cross-model RestoreTurn = (%T, %v), want checkpoint loss", mismatched, err)
	}
	if mismatched, err := eng2.RestoreTurn(context.Background(), proc.ID(), RestoreTurnRequest{
		SessionID:      wantScope.SessionID,
		ModelSelection: wantSelection,
		Cwd:            "/another/workspace",
		Isolated:       wantScope.Isolated,
		GoalLeaseID:    wantScope.GoalLeaseID,
		Limits:         wantLimits,
		Observer:       observer,
	}); mismatched != nil || !errors.Is(err, ErrExecutorCheckpointLost) {
		t.Fatalf("cross-workspace RestoreTurn = (%T, %v), want checkpoint loss", mismatched, err)
	}
	if mismatched, err := eng2.RestoreTurn(context.Background(), proc.ID(), RestoreTurnRequest{
		SessionID:      wantScope.SessionID,
		ModelSelection: wantSelection,
		Cwd:            wantScope.Cwd,
		WorkspaceCwd:   wantScope.WorkspaceCwd,
		Isolated:       wantScope.Isolated,
		GoalLeaseID:    "another-goal-lease",
		Limits:         wantLimits,
		Observer:       observer,
	}); mismatched != nil || !errors.Is(err, ErrExecutorCheckpointLost) {
		t.Fatalf("cross-goal RestoreTurn = (%T, %v), want checkpoint loss", mismatched, err)
	}

	restored, err := eng2.RestoreTurn(context.Background(), proc.ID(), RestoreTurnRequest{
		SessionID:      wantScope.SessionID,
		ModelSelection: wantSelection,
		Cwd:            wantScope.Cwd,
		WorkspaceCwd:   wantScope.WorkspaceCwd,
		Isolated:       wantScope.Isolated,
		GoalLeaseID:    wantScope.GoalLeaseID,
		Limits:         wantLimits,
		Observer:       observer,
	})
	if err != nil {
		t.Fatalf("RestoreTurn: %v", err)
	}
	restoredProcess, ok := restored.(*turnProcess)
	if !ok {
		t.Fatalf("restored process = %T, want *turnProcess", restored)
	}
	if scope, ok := executionctx.Scope(restoredProcess.runCtx); !ok || scope != wantScope {
		t.Fatalf("restored run scope = (%+v, %v), want %+v", scope, ok, wantScope)
	}
	pendingSuspensions, err := restored.PendingSuspensions(context.Background())
	if err != nil {
		t.Fatalf("PendingSuspensions: %v", err)
	}
	answers := make([]SuspensionAnswer, len(pendingSuspensions))
	for index, boundary := range pendingSuspensions {
		answers[index] = SuspensionAnswer{
			ProcessID:    boundary.ProcessID,
			SuspensionID: boundary.SuspensionID,
			Resolution:   interrupts.Resolution{Approved: true},
		}
	}
	if err := restored.Resume(context.Background(), answers); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	resumed := restored.Await()
	if err := resumed.Err; err != nil {
		t.Fatalf("resumed run: %v", err)
	}
	if !resumed.HasOutput {
		t.Fatal("resumed run completed without output")
	}
	out := resumed.Output
	if out.Reply != "restored ok" {
		t.Fatalf("reply = %q, want restored ok", out.Reply)
	}
	if out.CostUSD != 0.5 || len(out.UsageByModel) != 1 ||
		out.UsageByModel[0].Model != "unknown" || out.UsageByModel[0].Calls != 2 {
		t.Fatalf("restored usage = %+v cost=%v, want both pre-crash and resumed calls", out.UsageByModel, out.CostUSD)
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

func TestEngine_RestoreTurnRejectsDifferentExecutableBuild(t *testing.T) {
	stub := newOptionToolStub()
	client, _ := chatclient.New(stub, chatclient.Config{Defaults: *stub.defaults})
	store := newMemoryCheckpointStore()
	built, err := toolset.Build(t.Context(), testToolsetBuildConfig(t, toolset.BuildConfig{}))
	if err != nil {
		t.Fatalf("toolset.Build: %v", err)
	}
	cleanupBuiltTools(t, built)
	config := Config{
		ChatClient:   client,
		ToolResolver: built.Resolver,
		Checkpoints:  store,
		BuildID:      testBuildID,
	}
	first, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("first New: %v", err)
	}

	process, err := first.StartTurn(t.Context(), TurnRequest{
		SessionID: "session-build",
		Cwd:       "/workspace/build",
		Message:   "pause for approval",
		Observer:  &hitlApprovalObserver{},
	})
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if completion := process.Await(); completion.Err != nil {
		t.Fatalf("initial turn: %v", completion.Err)
	}
	persistWaitingCheckpoint(t, store, process)
	checkpoint, err := store.LoadCheckpoint(t.Context(), process.ID())
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if checkpoint.BuildID != testBuildID {
		t.Fatalf("checkpoint build = %q, want %q", checkpoint.BuildID, testBuildID)
	}
	frameworkTree, err := decodeValidatedProcessTree(checkpoint)
	if err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	snapshot, ok := frameworkTree.Root()
	if !ok {
		t.Fatalf("snapshot tree has no root %q: %+v", process.ID(), frameworkTree)
	}
	if snapshot.Deployment.Version != "" {
		t.Fatalf("snapshot display version = %q, want empty application agent version", snapshot.Deployment.Version)
	}

	config.BuildID = alternateBuildID
	second, err := New(t.Context(), config)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	resumable, err := second.CanResumeCheckpoint(t.Context(), expectationForCheckpoint(checkpoint))
	if err != nil {
		t.Fatalf("CanResumeCheckpoint: %v", err)
	}
	if resumable {
		t.Fatal("checkpoint from another executable build reported resumable")
	}

	restored, err := second.RestoreTurn(t.Context(), process.ID(), RestoreTurnRequest{
		SessionID:      checkpoint.Scope.SessionID,
		ModelSelection: checkpoint.ModelSelection,
		Cwd:            checkpoint.Scope.Cwd,
		Isolated:       checkpoint.Scope.Isolated,
		GoalLeaseID:    checkpoint.Scope.GoalLeaseID,
		Limits:         checkpoint.Limits,
		Observer:       &hitlApprovalObserver{},
	})
	if restored != nil {
		t.Fatalf("RestoreTurn process = %T, want nil", restored)
	}
	if !errors.Is(err, ErrExecutorCheckpointLost) {
		t.Fatalf("RestoreTurn error = %v, want ErrExecutorCheckpointLost", err)
	}
}

// TestEngine_RunChat_PerRunClientOverride verifies TurnRequest.ChatClient
// actually drives the turn's LLM call (via the ChatProvider seam),
// not the engine's default client.
func TestEngine_RunChat_PerRunClientOverride(t *testing.T) {
	defClient, _ := chatclient.New(newNamedStub("default-model"), chatclient.Config{})
	ovrClient, _ := chatclient.New(newNamedStub("override-model"), chatclient.Config{})
	eng, err := New(context.Background(), Config{ChatClient: defClient})
	if err != nil {
		t.Fatal(err)
	}
	out, err := eng.runTurnSync(context.Background(), TurnRequest{Message: "go", ChatClient: ovrClient})
	if err != nil {
		t.Fatalf("runTurnSync: %v", err)
	}
	if len(out.UsageByModel) != 1 || out.UsageByModel[0].Model != "override-model" {
		t.Fatalf("UsageByModel = %+v, want served model override-model", out.UsageByModel)
	}
}
