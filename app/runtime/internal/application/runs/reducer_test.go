package runs

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

func testReducerConfig() reducerConfig {
	now := time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
	return reducerConfig{
		RunID: "run_1", SegmentID: "seg_1", SessionID: "ses_1", Cwd: "/work",
		TurnID: "turn_1", Provider: "anthropic", Model: "claude", CreatedAt: now,
		Now: func() time.Time { return now },
	}
}

func TestReducerOpeningCreatesCanonicalRunAndUserItem(t *testing.T) {
	config := testReducerConfig()
	config.UserInput = []ContentBlock{{Kind: TextContent, Text: "hello"}}
	reducer := newReducer(config)

	opening := reducer.open()
	if len(opening) != 3 {
		t.Fatalf("opening reductions = %d, want segment + user item pair", len(opening))
	}
	started, ok := opening[0].Event.(SegmentStarted)
	if !ok || started.Run.ID != "run_1" || started.Run.SessionID != "ses_1" || started.Run.Model != "claude" {
		t.Fatalf("opening run = %#v", opening[0].Event)
	}
	itemStarted, ok := opening[1].Event.(ItemStarted)
	if !ok || itemStarted.Item.Kind != UserMessage || itemStarted.Item.Status != ItemRunning {
		t.Fatalf("user item start = %#v", opening[1].Event)
	}
	itemCompleted, ok := opening[2].Event.(ItemCompleted)
	if !ok || itemCompleted.Item.ID != itemStarted.Item.ID || itemCompleted.Item.SessionID != "ses_1" || itemCompleted.Item.Content[0].Text != "hello" {
		t.Fatalf("user item completion = %#v", opening[2].Event)
	}
	if opening[2].Commit == nil || len(opening[2].Commit.Items) != 1 {
		t.Fatal("completed user item has no canonical durable fact")
	}
	if again := reducer.open(); len(again) != 1 {
		t.Fatalf("second opening repeated user input: %+v", again)
	}
}

func TestReducerPreservesRawToolResultsAndExplicitFileNudges(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	reducer.reduce(ToolCallStart{CallID: "shell_1", ToolName: "shell", Arguments: `{"command":"echo hi"}`})
	raw := map[string]any{"stdout": "hi\n", "stderr": "oops", "exit_code": 0}
	reduced := reducer.reduce(ToolCallEnd{
		CallID: "shell_1", Result: raw, OutputText: "hi\n\noops",
	})
	completed := completedItem(t, reduced)
	if completed.Tool == nil {
		t.Fatal("completed tool is nil")
	}
	result, ok := completed.Tool.Result.(map[string]any)
	if !ok || result["stdout"] != "hi\n" || result["stderr"] != "oops" || result["exit_code"] != 0 {
		t.Fatalf("raw command result = %#v", completed.Tool.Result)
	}

	reducer.reduce(ToolCallStart{CallID: "write_1", ToolName: "write", Arguments: `{"file_path":"src/a.go"}`})
	write := reducer.reduce(ToolCallEnd{
		CallID: "write_1", Result: map[string]any{}, MutatedPaths: []string{"src/a.go"},
	})
	var nudge *Nudge
	for _, reduction := range write {
		if reduction.Nudge != nil {
			nudge = reduction.Nudge
		}
	}
	if nudge == nil || nudge.Cwd != "/work" || len(nudge.Paths) != 1 || nudge.Paths[0] != "src/a.go" {
		t.Fatalf("write nudge = %+v", nudge)
	}

	reducer.reduce(ToolCallStart{CallID: "denied_1", ToolName: "shell", Arguments: `{}`})
	denied := completedItem(t, reducer.reduce(ToolCallEnd{CallID: "denied_1", Denied: true}))
	if denied.Status != ItemIncomplete || denied.Error == nil || denied.Error.Kind != DeniedByUserProblem {
		t.Fatalf("denied item = %+v", denied)
	}
}

func TestReducerCanonicalProgressSnapshotsAndOutcomes(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	usage := reducer.reduce(UsageReported{
		TokenUsage: accounting.TokenUsage{PromptTokens: 1200, CompletionTokens: 80, ReasoningTokens: 30},
		CostUSD:    0.0125, ContextTokens: 4096,
	})
	progress, ok := usage[0].Event.(SegmentProgressed)
	if !ok || progress.Progress.Usage == nil || progress.Progress.Usage.InputTokens != 1200 || progress.Progress.Usage.CostUSD == nil {
		t.Fatalf("usage progress = %#v", usage[0].Event)
	}
	if usage[0].Commit != nil {
		t.Fatal("usage progress must stay ephemeral")
	}

	snapshot := reducer.reduce(TodosUpdated{Todos: []todo.Item{{
		Content: "write tests", Status: todo.StatusInProgress, NextAction: "run package",
	}}})
	state, ok := snapshot[0].Event.(StateSnapshot)
	if !ok || len(state.Todos) != 1 || state.Todos[0].Text != "write tests" || state.Todos[0].Status != "in_progress" {
		t.Fatalf("todo snapshot = %#v", snapshot[0].Event)
	}

	compaction := reducer.reduce(CompactBoundary{MessagesBefore: 20, MessagesAfter: 6})
	if item := completedItem(t, compaction); item.Kind != Compaction || item.DroppedMessages != 14 {
		t.Fatalf("compaction item = %+v", item)
	}

	terminal := reducer.reduce(TurnEnd{
		Reason: execution.OutcomeMaxBudget, Duration: 1500 * time.Millisecond,
		CostUSD: 4.2, MaxCostUSD: 4,
	})
	finished := terminal[len(terminal)-1].Event.(SegmentFinished)
	if finished.Run.Result == nil || finished.Run.Result.Duration != 1500*time.Millisecond || !strings.Contains(finished.Run.Detail, "$4.20") {
		t.Fatalf("budget terminal = %+v", finished.Run)
	}
}

func TestReducerClassifiesErrorsWithoutLeakingProviderDetails(t *testing.T) {
	cases := []struct {
		message string
		kind    ProblemKind
		retry   bool
	}{
		{`POST "https://api.example": 429 Too Many Requests; retry-after: 30`, RateLimitedProblem, true},
		{`POST "https://api.example": 401 Unauthorized`, InvalidAPIKeyProblem, false},
		{`POST "https://api.example": 503 Service Unavailable`, ProviderUnavailableProblem, true},
		{`POST "https://api.example": context deadline exceeded`, TimeoutProblem, true},
		{`POST "https://api.example": 400 Bad Request`, ProviderRejectedProblem, false},
	}
	for _, test := range cases {
		reducer := newReducer(testReducerConfig())
		problem := reducer.classifyRunError(test.message)
		if problem.Kind != test.kind || problem.Retryable != test.retry || strings.Contains(problem.Detail, "api.example") {
			t.Errorf("classify(%q) = %+v", test.message, problem)
		}
	}
	reducer := newReducer(testReducerConfig())
	reducer.errCode = "AGENT_STUCK"
	if problem := reducer.classifyRunError("no progress"); problem.Kind != AgentStuckProblem {
		t.Fatalf("agent stuck problem = %+v", problem)
	}
	if got := parseRetryAfter("try again in 12 seconds"); got != 12 {
		t.Fatalf("retry-after = %d, want 12", got)
	}
}

func TestReducerResumeReusesInterruptedItems(t *testing.T) {
	question := &transcript.Question{Prompt: "Continue?"}
	config := testReducerConfig()
	config.Pending = &interrupts.Pending{
		RunID: "run_1", SessionID: "ses_1",
		Interrupts: []transcript.Interrupt{
			{ItemID: "item_approval", Kind: transcript.ApprovalInterrupt, Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Arguments: map[string]any{"command": "go test"}}}},
			{ItemID: "item_question", Kind: transcript.QuestionInterrupt, Question: question},
		},
	}
	reducer := newReducer(config)
	opening := reducer.open()
	completed, ok := opening[len(opening)-1].Event.(ItemCompleted)
	if !ok || completed.Item.ID != "item_question" || completed.Item.Question != question {
		t.Fatalf("resumed question completion = %#v", opening[len(opening)-1].Event)
	}

	started := reducer.reduce(ToolCallStart{CallID: "call_1", ToolName: "shell", Arguments: `{"command":"go test"}`})
	var itemID string
	for _, reduction := range started {
		if event, ok := reduction.Event.(ItemStarted); ok {
			itemID = event.Item.ID
		}
	}
	if itemID != "item_approval" {
		t.Fatalf("resumed tool item id = %q, want item_approval", itemID)
	}
	reducer.reduce(ToolCallEnd{CallID: "call_1", Result: "ok"})

	second := reducer.reduce(ToolCallStart{CallID: "call_2", ToolName: "shell", Arguments: `{"command":"go vet"}`})
	var secondID string
	for _, reduction := range second {
		if event, ok := reduction.Event.(ItemStarted); ok {
			secondID = event.Item.ID
		}
	}
	if secondID == "" || secondID == "item_approval" {
		t.Fatalf("new same-name tool item id = %q, want a fresh identity", secondID)
	}
}

func TestReducerProjectsParkAsOneAtomicWriteSetBeforeFirstInterruptEvent(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	reduced := reducer.reduce(TurnInterrupted{Interrupts: []Interrupt{
		{Kind: ApprovalInterruptKind, Approval: &ApprovalPrompt{ToolName: "shell", Arguments: `{}`}},
		{Kind: QuestionInterruptKind, Question: &interrupts.QuestionPrompt{Questions: []interrupts.Question{{Question: "Continue?"}}}},
	}})

	interruptReduction := -1
	for i, reduction := range reduced {
		if reduction.Interrupt {
			if interruptReduction >= 0 {
				t.Fatal("park has more than one atomic commit boundary")
			}
			interruptReduction = i
		}
	}
	if interruptReduction < 0 {
		t.Fatal("park has no atomic commit boundary")
	}
	first, ok := reduced[interruptReduction].Event.(ItemStarted)
	if !ok || first.Item.SessionID != "ses_1" || interruptReduction != 0 {
		t.Fatalf("atomic boundary event = %#v at %d, want first persisted interrupt item at batch start", reduced[interruptReduction].Event, interruptReduction)
	}
	commit := reduced[interruptReduction].Commit
	if commit == nil || len(commit.Items) != 2 || commit.Run == nil || commit.Interrupt == nil || commit.State != StateSuspend {
		t.Fatalf("park commit = %+v, want items + run + interrupt + suspend", commit)
	}
	for _, item := range commit.Items {
		if item.SessionID != "ses_1" || item.RunID != "run_1" || item.Status != ItemRunning {
			t.Fatalf("persisted interrupt item = %+v", item)
		}
	}
	if terminal := reduced[len(reduced)-1]; terminal.Commit != nil || terminal.Interrupt {
		t.Fatalf("terminal event repeated park commit: %+v", terminal)
	}
}

func TestReducerDrainsToolsInStartOrder(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	for _, event := range []ToolCallStart{
		{CallID: "call_z", ToolName: "first", Arguments: `{}`},
		{CallID: "call_a", ToolName: "second", Arguments: `{}`},
		{CallID: "call_m", ToolName: "third", Arguments: `{}`},
	} {
		reducer.reduce(event)
	}

	drained := reducer.tools.snapshot()
	if len(drained) != 3 {
		t.Fatalf("drained tool count = %d, want 3", len(drained))
	}
	if got := []string{drained[0].Name, drained[1].Name, drained[2].Name}; !slices.Equal(got, []string{"first", "second", "third"}) {
		t.Fatalf("drained tools = %v, want start order", got)
	}
	completed := reducer.drainTools()
	got := make([]string, 0, len(completed))
	for _, event := range completed {
		got = append(got, event.(ItemCompleted).Item.Tool.Name)
	}
	if !slices.Equal(got, []string{"first", "second", "third"}) {
		t.Fatalf("completed tools = %v, want start order", got)
	}
	if len(reducer.tools) != 0 {
		t.Fatalf("open tools after drain = %d, want 0", len(reducer.tools))
	}
}

func completedItem(t *testing.T, reductions []reduction) Item {
	t.Helper()
	for _, reduction := range reductions {
		if event, ok := reduction.Event.(ItemCompleted); ok {
			return event.Item
		}
	}
	t.Fatalf("no ItemCompleted in %+v", reductions)
	return Item{}
}
