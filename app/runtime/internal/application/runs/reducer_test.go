package runs

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func testReducerConfig() reducerConfig {
	now := time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
	return reducerConfig{
		RunID: "run_1", SegmentID: "seg_1", SessionID: "ses_1", Cwd: "/work",
		TurnID: "turn_1", Provider: "anthropic", Model: "claude", CreatedAt: now,
		Now: func() time.Time { return now },
	}
}

type unsupportedEngineEvent struct{ engineEventBase }

func mustOpen(t *testing.T, reducer *reducer) []reduction {
	t.Helper()
	reductions, err := reducer.open()
	if err != nil {
		t.Fatalf("open reducer: %v", err)
	}
	return reductions
}

func mustReduce(t *testing.T, reducer *reducer, event EngineEvent) []reduction {
	t.Helper()
	reductions, err := reducer.reduce(event)
	if err != nil {
		t.Fatalf("reduce %T: %v", event, err)
	}
	return reductions
}

func TestReducerOpeningCreatesCanonicalRunAndUserItem(t *testing.T) {
	config := testReducerConfig()
	config.UserInput = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}
	reducer := newReducer(config)

	opening := mustOpen(t, reducer)
	if len(opening) != 3 {
		t.Fatalf("opening reductions = %d, want segment + user item pair", len(opening))
	}
	started, ok := opening[0].Event.(SegmentStarted)
	if !ok || started.Run.ID != "run_1" || started.Run.SessionID != "ses_1" || started.Run.Model != "claude" {
		t.Fatalf("opening run = %#v", opening[0].Event)
	}
	itemStarted, ok := opening[1].Event.(ItemStarted)
	if !ok || itemStarted.Item.Kind != transcript.UserMessage || itemStarted.Item.Status != transcript.ItemRunning {
		t.Fatalf("user item start = %#v", opening[1].Event)
	}
	itemCompleted, ok := opening[2].Event.(ItemCompleted)
	if !ok || itemCompleted.Item.ID != itemStarted.Item.ID || itemCompleted.Item.SessionID != "ses_1" || itemCompleted.Item.Content[0].Text != "hello" {
		t.Fatalf("user item completion = %#v", opening[2].Event)
	}
	if opening[2].Commit == nil || len(opening[2].Commit.Items) != 1 {
		t.Fatal("completed user item has no canonical durable fact")
	}
	if again := mustOpen(t, reducer); len(again) != 1 {
		t.Fatalf("second opening repeated user input: %+v", again)
	}
}

func TestReducerOwnsOpeningUserInput(t *testing.T) {
	config := testReducerConfig()
	config.UserInput = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "original"}}
	reducer := newReducer(config)

	config.UserInput[0].Text = "reused by caller"
	opening := mustOpen(t, reducer)
	completed, ok := opening[2].Event.(ItemCompleted)
	if !ok || len(completed.Item.Content) != 1 || completed.Item.Content[0].Text != "original" {
		t.Fatalf("opening user item = %#v, want owned original input", opening[2].Event)
	}
}

func TestReducerPreservesRawToolResultsAndExplicitFileNudges(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, ToolCallStart{CallID: "shell_1", ToolName: "shell", Arguments: `{"command":"echo hi"}`})
	raw := map[string]any{"stdout": "hi\n", "stderr": "oops", "exit_code": 0}
	reduced := mustReduce(t, reducer, ToolCallEnd{
		CallID: "shell_1", Result: testToolResult(t, raw), OutputText: "hi\n\noops",
	})
	completed := completedItem(t, reduced)
	if completed.Tool == nil {
		t.Fatal("completed tool is nil")
	}
	result, ok := completed.Tool.Result.Any().(map[string]any)
	if !ok || result["stdout"] != "hi\n" || result["stderr"] != "oops" || result["exit_code"] != json.Number("0") {
		t.Fatalf("raw command result = %#v", completed.Tool.Result)
	}

	mustReduce(t, reducer, ToolCallStart{CallID: "write_1", ToolName: "write", Arguments: `{"file_path":"src/a.go"}`})
	write := mustReduce(t, reducer, ToolCallEnd{
		CallID: "write_1", Result: testToolResult(t, map[string]any{}), MutatedPaths: []string{"src/a.go"},
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

	mustReduce(t, reducer, ToolCallStart{CallID: "denied_1", ToolName: "shell", Arguments: `{}`})
	denied := completedItem(t, mustReduce(t, reducer, ToolCallEnd{CallID: "denied_1", Denied: true}))
	if denied.Status != transcript.ItemIncomplete || denied.Error == nil || denied.Error.Kind != transcript.DeniedByUserProblem {
		t.Fatalf("denied item = %+v", denied)
	}
}

func TestReducerCommitsConcurrentToolCompletionsInModelOrder(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	for _, event := range []ToolCallStart{
		{CallID: "call-1", ToolName: "first", Arguments: `{"value":1}`},
		{CallID: "call-2", ToolName: "second", Arguments: `{"value":2}`},
		{CallID: "call-3", ToolName: "third", Arguments: `{"value":3}`},
	} {
		mustReduce(t, reducer, event)
	}

	if reduced := mustReduce(t, reducer, ToolCallEnd{CallID: "call-3", Result: testToolResult(t, "three")}); len(reduced) != 0 {
		t.Fatalf("third completion escaped ordering barrier: %+v", reduced)
	}
	first := mustReduce(t, reducer, ToolCallEnd{CallID: "call-1", Result: testToolResult(t, "one")})
	if got := completedToolNames(first); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("first completion batch = %v, want [first]", got)
	}
	remaining := mustReduce(t, reducer, ToolCallEnd{
		CallID: "call-2", Arguments: `{"value":20}`, Result: testToolResult(t, "two"),
	})
	if got := completedToolNames(remaining); !slices.Equal(got, []string{"second", "third"}) {
		t.Fatalf("released completion batch = %v, want [second third]", got)
	}
	second := completedItem(t, remaining)
	if second.Tool.Arguments.Map()["value"] != json.Number("20") {
		t.Fatalf("effective arguments = %#v, want value 20", second.Tool.Arguments)
	}
}

func TestReducerParksConcurrentToolsWithoutLosingCompletedResults(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	firstStart := mustReduce(t, reducer, ToolCallStart{
		CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
	})
	firstID := startedItemID(t, firstStart)
	mustReduce(t, reducer, ToolCallStart{
		CallID: "call-2", ToolName: "lookup", Arguments: `{"path":"b"}`, SafetyClass: "read",
	})
	if reduced := mustReduce(t, reducer, ToolCallEnd{CallID: "call-2", Result: testToolResult(t, "found")}); len(reduced) != 0 {
		t.Fatalf("later completion escaped paused prefix: %+v", reduced)
	}

	parked := mustReduce(t, reducer, TurnInterrupted{Interrupts: []Interrupt{{
		Kind: ApprovalInterruptKind,
		Approval: &ApprovalPrompt{
			CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
		},
	}}})
	commit := parked[0].Commit
	if commit == nil || commit.Interrupt == nil || len(commit.Items) != 2 {
		t.Fatalf("park commit = %+v, want two ordered tool items", commit)
	}
	if commit.Items[0].ID != firstID || commit.Items[0].Status != transcript.ItemRunning {
		t.Fatalf("active approval item = %+v, want original running item %q", commit.Items[0], firstID)
	}
	if commit.Items[1].Tool == nil || commit.Items[1].Tool.Result == nil {
		t.Fatalf("completed sibling item = %+v", commit.Items[1])
	}
	result, ok := commit.Items[1].Tool.Result.String()
	if commit.Items[1].Tool.Name != "lookup" ||
		commit.Items[1].Status != transcript.ItemCompleted || !ok || result != "found" {
		t.Fatalf("completed sibling item = %+v", commit.Items[1])
	}
	if got := commit.Interrupt.Interrupts[0].ItemID; got != firstID {
		t.Fatalf("approval item ID = %q, want original %q", got, firstID)
	}
	if len(commit.Interrupt.DrainedTools) != 0 {
		t.Fatalf("completed or active approval leaked into drained tools: %+v", commit.Interrupt.DrainedTools)
	}
}

func TestReducerCarriesLaterPausedCallIdentityAcrossSequentialResumes(t *testing.T) {
	first := newReducer(testReducerConfig())
	firstID := startedItemID(t, mustReduce(t, first, ToolCallStart{
		CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
	}))
	secondID := startedItemID(t, mustReduce(t, first, ToolCallStart{
		CallID: "call-2", ToolName: "approval", Arguments: `{"path":"b"}`, SafetyClass: "write",
	}))
	firstPark := mustReduce(t, first, TurnInterrupted{Interrupts: []Interrupt{{
		Kind: ApprovalInterruptKind,
		Approval: &ApprovalPrompt{
			CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
		},
	}}})[0].Commit.Interrupt
	if firstPark.Interrupts[0].ItemID != firstID || len(firstPark.DrainedTools) != 1 ||
		firstPark.DrainedTools[0].ItemID != secondID || firstPark.DrainedTools[0].CallID != "call-2" {
		t.Fatalf("first park identity state = %+v", firstPark)
	}

	config := testReducerConfig()
	config.SegmentID = "seg_2"
	config.Pending = firstPark
	resumed := newReducer(config)
	mustOpen(t, resumed)
	if got := startedItemID(t, mustReduce(t, resumed, ToolCallStart{
		CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
	})); got != firstID {
		t.Fatalf("resumed first item ID = %q, want %q", got, firstID)
	}
	mustReduce(t, resumed, ToolCallEnd{CallID: "call-1", Result: testToolResult(t, "approved")})

	secondPark := mustReduce(t, resumed, TurnInterrupted{Interrupts: []Interrupt{{
		Kind: ApprovalInterruptKind,
		Approval: &ApprovalPrompt{
			CallID: "call-2", ToolName: "approval", Arguments: `{"path":"b"}`, SafetyClass: "write",
		},
	}}})[0].Commit.Interrupt
	if got := secondPark.Interrupts[0].ItemID; got != secondID {
		t.Fatalf("later approval item ID = %q, want original %q", got, secondID)
	}
	if len(secondPark.DrainedTools) != 0 {
		t.Fatalf("surfaced later approval remained drained: %+v", secondPark.DrainedTools)
	}
}

func TestReducerCanonicalProgressSnapshotsAndOutcomes(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	usage := mustReduce(t, reducer, UsageReported{
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

	snapshot := mustReduce(t, reducer, TodosUpdated{Todos: []todo.Item{{
		Content: "write tests", Status: todo.StatusInProgress, NextAction: "run package",
	}}})
	state, ok := snapshot[0].Event.(StateSnapshot)
	if !ok || len(state.Todos) != 1 || state.Todos[0].Text != "write tests" || state.Todos[0].Status != "in_progress" {
		t.Fatalf("todo snapshot = %#v", snapshot[0].Event)
	}

	compaction := mustReduce(t, reducer, CompactBoundary{MessagesBefore: 20, MessagesAfter: 6})
	if item := completedItem(t, compaction); item.Kind != transcript.Compaction || item.DroppedMessages != 14 {
		t.Fatalf("compaction item = %+v", item)
	}

	terminal := mustReduce(t, reducer, TurnEnd{
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
		name    string
		problem transcript.Problem
	}{
		{"rate limited", transcript.Problem{Kind: transcript.RateLimitedProblem, Detail: "retry shortly", Retryable: true, RetryAfterSeconds: 30}},
		{"invalid credentials", transcript.Problem{Kind: transcript.InvalidAPIKeyProblem, Detail: "check credentials"}},
		{"provider unavailable", transcript.Problem{Kind: transcript.ProviderUnavailableProblem, Detail: "retry shortly", Retryable: true}},
		{"timeout", transcript.Problem{Kind: transcript.TimeoutProblem, Detail: "retry shortly", Retryable: true}},
		{"provider rejected", transcript.Problem{Kind: transcript.ProviderRejectedProblem, Detail: "invalid request"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reducer := newReducer(testReducerConfig())
			mustReduce(t, reducer, ErrorEvent{
				Message: `POST "https://api.example": provider detail`,
				Code:    ErrorCodeEngine, Problem: test.problem,
			})
			terminal := mustReduce(t, reducer, TurnEnd{Reason: execution.OutcomeError})
			finished := terminal[len(terminal)-1].Event.(SegmentFinished)
			problem := finished.Run.Result.Error
			if problem == nil || *problem != (transcript.Problem{
				Kind: test.problem.Kind, Scope: transcript.RunProblem, Detail: test.problem.Detail,
				Retryable: test.problem.Retryable, RetryAfterSeconds: test.problem.RetryAfterSeconds,
			}) || strings.Contains(problem.Detail, "api.example") {
				t.Errorf("terminal problem = %+v", problem)
			}
		})
	}
}

func TestReducerResumeReusesInterruptedItems(t *testing.T) {
	question := &transcript.Question{Prompt: "Continue?"}
	config := testReducerConfig()
	config.Pending = &interrupts.Pending{
		RunID: "run_1", SessionID: "ses_1",
		Interrupts: []transcript.Interrupt{
			{ItemID: "item_approval", Kind: transcript.ApprovalInterrupt, Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Arguments: testToolArguments(t, map[string]any{"command": "go test"})}}},
			{ItemID: "item_question", Kind: transcript.QuestionInterrupt, Question: question},
		},
	}
	reducer := newReducer(config)
	opening := mustOpen(t, reducer)
	completed, ok := opening[len(opening)-1].Event.(ItemCompleted)
	if !ok || completed.Item.ID != "item_question" || completed.Item.Question != question {
		t.Fatalf("resumed question completion = %#v", opening[len(opening)-1].Event)
	}

	started := mustReduce(t, reducer, ToolCallStart{CallID: "call_1", ToolName: "shell", Arguments: `{"command":"go test"}`})
	var itemID string
	for _, reduction := range started {
		if event, ok := reduction.Event.(ItemStarted); ok {
			itemID = event.Item.ID
		}
	}
	if itemID != "item_approval" {
		t.Fatalf("resumed tool item id = %q, want item_approval", itemID)
	}
	mustReduce(t, reducer, ToolCallEnd{CallID: "call_1", Result: testToolResult(t, "ok")})

	second := mustReduce(t, reducer, ToolCallStart{CallID: "call_2", ToolName: "shell", Arguments: `{"command":"go vet"}`})
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
	reduced := mustReduce(t, reducer, TurnInterrupted{Interrupts: []Interrupt{
		{Kind: ApprovalInterruptKind, Approval: &ApprovalPrompt{
			ToolName: "shell", Arguments: `{}`, SafetyClass: "exec",
		}},
		{Kind: QuestionInterruptKind, Question: &QuestionPrompt{
			ToolName: "ask_user", Arguments: `{"questions":[{"question":"Continue?"}]}`,
			Questions: []QuestionSpec{{Question: "Continue?"}},
		}},
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
		if item.SessionID != "ses_1" || item.RunID != "run_1" || item.Status != transcript.ItemRunning {
			t.Fatalf("persisted interrupt item = %+v", item)
		}
	}
	if terminal := reduced[len(reduced)-1]; terminal.Commit != nil || terminal.Interrupt {
		t.Fatalf("terminal event repeated park commit: %+v", terminal)
	}
}

func TestReducerRejectsExecutorProtocolViolations(t *testing.T) {
	tests := []struct {
		name  string
		event EngineEvent
	}{
		{name: "unknown event", event: unsupportedEngineEvent{}},
		{name: "invalid terminal outcome", event: TurnEnd{Reason: execution.Outcome(255)}},
		{name: "malformed interrupt", event: TurnInterrupted{Interrupts: []Interrupt{{Kind: InterruptKind("unknown")}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newReducer(testReducerConfig()).reduce(test.event)
			if !errors.Is(err, errExecutorProtocol) {
				t.Fatalf("reduce %T error = %v, want executor protocol violation", test.event, err)
			}
		})
	}
}

func TestReducerRejectsMalformedToolArguments(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		reducer := newReducer(testReducerConfig())
		_, err := reducer.reduce(ToolCallStart{
			CallID: "call_1", ToolName: "shell", Arguments: `{"command":`,
		})
		if !errors.Is(err, errExecutorProtocol) || !errors.Is(err, tool.ErrInvalidArguments) {
			t.Fatalf("tool start error = %v, want executor protocol + invalid arguments", err)
		}
		if len(reducer.tools) != 0 || reducer.step != 0 {
			t.Fatalf("malformed start mutated reducer: tools=%d step=%d", len(reducer.tools), reducer.step)
		}
	})

	t.Run("effective end arguments", func(t *testing.T) {
		reducer := newReducer(testReducerConfig())
		mustReduce(t, reducer, ToolCallStart{
			CallID: "call_1", ToolName: "shell", Arguments: `{"command":"go test"}`,
		})
		_, err := reducer.reduce(ToolCallEnd{CallID: "call_1", Arguments: "null"})
		if !errors.Is(err, errExecutorProtocol) || !errors.Is(err, tool.ErrInvalidArguments) {
			t.Fatalf("tool end error = %v, want executor protocol + invalid arguments", err)
		}
	})
}

func TestReducerRejectsInvalidToolLifecycle(t *testing.T) {
	tests := []struct {
		name  string
		event EngineEvent
		want  string
	}{
		{name: "missing call id", event: ToolCallStart{ToolName: "shell"}, want: "id is required"},
		{name: "missing tool name", event: ToolCallStart{CallID: "call_1"}, want: "name is required"},
		{name: "end without start", event: ToolCallEnd{CallID: "call_1"}, want: "without an open start"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newReducer(testReducerConfig()).reduce(test.event)
			if !errors.Is(err, errExecutorProtocol) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("reduce error = %v, want executor protocol containing %q", err, test.want)
			}
		})
	}

	t.Run("duplicate start", func(t *testing.T) {
		reducer := newReducer(testReducerConfig())
		start := ToolCallStart{CallID: "call_1", ToolName: "shell", Arguments: `{}`}
		mustReduce(t, reducer, start)
		_, err := reducer.reduce(start)
		if !errors.Is(err, errExecutorProtocol) || !strings.Contains(err.Error(), "started more than once") {
			t.Fatalf("duplicate start error = %v", err)
		}
	})
}

func TestReducerUsesCanonicalArgumentsForResumeIdentity(t *testing.T) {
	config := testReducerConfig()
	config.Pending = &interrupts.Pending{
		RunID: "run_1", SessionID: "ses_1",
		DrainedTools: []interrupts.DrainedTool{{
			ItemID: "item_original", CallID: "old_call", Name: "lookup",
			Arguments: `{"b":2,"a":{"enabled":true}}`,
		}},
	}
	reducer := newReducer(config)
	started := mustReduce(t, reducer, ToolCallStart{
		CallID: "new_call", ToolName: "lookup",
		Arguments: "{\n  \"a\": {\"enabled\": true}, \"b\": 2\n}",
	})
	if got := startedItemID(t, started); got != "item_original" {
		t.Fatalf("resumed item id = %q, want canonical match item_original", got)
	}
}

func TestReducerRejectsMalformedDurableResumeArguments(t *testing.T) {
	config := testReducerConfig()
	config.Pending = &interrupts.Pending{
		RunID: "run_1", SessionID: "ses_1",
		DrainedTools: []interrupts.DrainedTool{{
			ItemID: "item_broken", Name: "lookup", Arguments: "[]",
		}},
	}
	_, err := newReducer(config).open()
	if !errors.Is(err, errReducerInvariant) || !errors.Is(err, tool.ErrInvalidArguments) {
		t.Fatalf("open error = %v, want reducer invariant + invalid arguments", err)
	}
}

func TestReducerRejectsInvalidInterruptProjection(t *testing.T) {
	interrupted := SegmentFinished{Run: transcript.Run{State: execution.Interrupted}}
	tests := []struct {
		name   string
		events []RunEvent
	}{
		{
			name:   "multiple interrupt boundaries",
			events: []RunEvent{interrupted, interrupted},
		},
		{
			name: "additional lifecycle transition",
			events: []RunEvent{
				SegmentStarted{Run: transcript.Run{State: execution.Running}},
				interrupted,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newReducer(testReducerConfig()).project(test.events)
			if !errors.Is(err, errReducerInvariant) {
				t.Fatalf("project error = %v, want reducer invariant violation", err)
			}
		})
	}
}

func TestValidateReductionBatchRejectsMalformedBoundaries(t *testing.T) {
	interruptCommit := func() *EventCommit {
		run := transcript.Run{State: execution.Interrupted}
		return &EventCommit{
			State:     StateSuspend,
			Run:       &run,
			Interrupt: new(interrupts.Pending),
		}
	}
	terminalCommit := func() *EventCommit {
		outcome := execution.OutcomeCompleted
		run := transcript.Run{State: execution.Completed, Outcome: &outcome}
		return &EventCommit{
			State:   StateTerminalize,
			Outcome: outcome,
			Run:     &run,
		}
	}
	invalidTerminalCommit := terminalCommit()
	invalidTerminalCommit.Run.State = execution.Failed
	tests := []struct {
		name       string
		reductions []reduction
	}{
		{
			name:       "missing event",
			reductions: []reduction{{}},
		},
		{
			name: "terminal is not last",
			reductions: []reduction{
				{Event: SegmentFinished{}, Commit: terminalCommit()},
				{Event: SegmentProgressed{}},
			},
		},
		{
			name:       "terminal has no commit",
			reductions: []reduction{{Event: SegmentFinished{}}},
		},
		{
			name:       "terminal lifecycle is inconsistent",
			reductions: []reduction{{Event: SegmentFinished{}, Commit: invalidTerminalCommit}},
		},
		{
			name: "commit state is unknown",
			reductions: []reduction{{
				Event:  ItemCompleted{},
				Commit: &EventCommit{State: StateChange(255)},
			}},
		},
		{
			name: "interrupt marker is not first",
			reductions: []reduction{
				{Event: SegmentProgressed{}},
				{Event: SegmentFinished{}, Commit: interruptCommit(), Interrupt: true},
			},
		},
		{
			name:       "interrupt has no commit",
			reductions: []reduction{{Event: SegmentFinished{}, Interrupt: true}},
		},
		{
			name: "interrupt has no terminal event",
			reductions: []reduction{
				{Event: ItemStarted{}, Commit: interruptCommit(), Interrupt: true},
			},
		},
		{
			name: "interrupt repeats a commit",
			reductions: []reduction{
				{Event: ItemStarted{}, Commit: interruptCommit(), Interrupt: true},
				{Event: SegmentFinished{}, Commit: new(EventCommit)},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateReductionBatch(test.reductions); !errors.Is(err, errReducerInvariant) {
				t.Fatalf("validateReductionBatch error = %v, want reducer invariant violation", err)
			}
		})
	}
}

func TestReducerDrainsToolsInStartOrder(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	for _, event := range []ToolCallStart{
		{CallID: "call_z", ToolName: "first", Arguments: `{}`},
		{CallID: "call_a", ToolName: "second", Arguments: `{}`},
		{CallID: "call_m", ToolName: "third", Arguments: `{}`},
	} {
		mustReduce(t, reducer, event)
	}

	drained := drainedToolRefs(reducer.tools.ordered(), nil)
	if len(drained) != 3 {
		t.Fatalf("drained tool count = %d, want 3", len(drained))
	}
	if got := []string{drained[0].Name, drained[1].Name, drained[2].Name}; !slices.Equal(got, []string{"first", "second", "third"}) {
		t.Fatalf("drained tools = %v, want start order", got)
	}
	completed, err := reducer.drainTools()
	if err != nil {
		t.Fatalf("drainTools: %v", err)
	}
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

func testToolResult(t *testing.T, value any) *tool.Result {
	t.Helper()
	result, err := tool.NewResult(value)
	if err != nil {
		t.Fatalf("NewResult: %v", err)
	}
	return &result
}

func testToolArguments(t *testing.T, value map[string]any) tool.Arguments {
	t.Helper()
	result, err := tool.ArgumentsFromMap(value)
	if err != nil {
		t.Fatalf("ArgumentsFromMap: %v", err)
	}
	return result
}

func completedItem(t *testing.T, reductions []reduction) transcript.Item {
	t.Helper()
	for _, reduction := range reductions {
		if event, ok := reduction.Event.(ItemCompleted); ok {
			return event.Item
		}
	}
	t.Fatalf("no ItemCompleted in %+v", reductions)
	return transcript.Item{}
}

func startedItemID(t *testing.T, reductions []reduction) string {
	t.Helper()
	for _, reduction := range reductions {
		if event, ok := reduction.Event.(ItemStarted); ok {
			return event.Item.ID
		}
	}
	t.Fatalf("no ItemStarted in %+v", reductions)
	return ""
}

func completedToolNames(reductions []reduction) []string {
	var names []string
	for _, reduction := range reductions {
		if event, ok := reduction.Event.(ItemCompleted); ok && event.Item.Tool != nil {
			names = append(names, event.Item.Tool.Name)
		}
	}
	return names
}
