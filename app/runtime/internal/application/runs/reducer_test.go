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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func testReducerConfig() reducerConfig {
	now := time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
	return reducerConfig{
		RunID: "run_1", SegmentID: "seg_1", SessionID: "ses_1", Cwd: "/work",
		TurnID: "turn_1", ModelSelection: mustReducerSelection("anthropic", "claude"), CreatedAt: now,
		Now: func() time.Time { return now },
	}
}

func TestReducerTerminalIncludesGoalTurnRecord(t *testing.T) {
	config := testReducerConfig()
	config.GoalLeaseID = "goal_lease"
	reducer := newReducer(config)
	mustReduce(t, reducer, ToolCallStart{CallID: "call_1", ToolName: "inspect", Arguments: `{}`})
	mustReduce(t, reducer, ToolCallEnd{CallID: "call_1", Result: testToolResult(t, "ok")})
	reductions := mustReduce(t, reducer, TurnEnd{
		Reason: execution.OutcomeCompleted,
		Usage:  &TurnUsage{CostUSD: 0.75, Steps: 1},
	})
	commit := reductions[len(reductions)-1].Commit
	if commit == nil || commit.GoalTurn == nil {
		t.Fatal("terminal commit did not carry goal turn accounting")
	}
	want := goal.TurnRecord{SessionID: "ses_1", LeaseID: "goal_lease", RunID: "run_1", Outcome: execution.OutcomeCompleted, CostUSD: 0.75, Steps: 1, CompletedAt: config.Now()}
	if got := *commit.GoalTurn; got != want {
		t.Fatalf("GoalTurn = %+v", got)
	}
}

func TestReducerStepsCountModelCallsRatherThanParallelTools(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, UsageReported{Steps: 1})
	mustReduce(t, reducer, ToolCallStart{CallID: "call_1", ToolName: "inspect", Arguments: `{}`})
	mustReduce(t, reducer, ToolCallStart{CallID: "call_2", ToolName: "inspect", Arguments: `{}`})
	mustReduce(t, reducer, ToolCallEnd{CallID: "call_2", Result: testToolResult(t, "two")})
	mustReduce(t, reducer, ToolCallEnd{CallID: "call_1", Result: testToolResult(t, "one")})
	finished := mustReduce(t, reducer, TurnEnd{
		Reason: execution.OutcomeCompleted,
		Usage:  &TurnUsage{Steps: 1},
	})
	run := finished[len(finished)-1].Event.(SegmentFinished).Run
	if run.Metrics.Steps != 1 {
		t.Fatalf("steps = %d, want one model call for two parallel tools", run.Metrics.Steps)
	}
}

func TestReducerTreatsExecutorAccountingAsCumulativeAcrossResume(t *testing.T) {
	config := testReducerConfig()
	config.Metrics = transcript.RunMetrics{
		Usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{
			InputTokens: 10,
		}},
		Steps:          2,
		ActiveDuration: time.Second,
	}
	reducer := newReducer(config)
	mustReduce(t, reducer, UsageReported{
		TokenUsage: accounting.TokenUsage{PromptTokens: 15},
		Steps:      3,
	})
	finished := mustReduce(t, reducer, TurnEnd{
		Reason: execution.OutcomeCompleted,
		Usage: &TurnUsage{
			Tokens: accounting.TokenUsage{PromptTokens: 15},
			Steps:  3,
		},
		Duration: 2 * time.Second,
	})
	run := finished[len(finished)-1].Event.(SegmentFinished).Run
	if run.Metrics.Steps != 3 ||
		run.Metrics.Usage == nil ||
		run.Metrics.Usage.InputTokens != 15 ||
		run.Metrics.ActiveDuration != 3*time.Second {
		t.Fatalf("cumulative metrics = %+v", run.Metrics)
	}
}

func TestReducerRejectsInconsistentOrRegressingAccounting(t *testing.T) {
	t.Run("step regression", func(t *testing.T) {
		config := testReducerConfig()
		config.Metrics.Steps = 2
		_, err := newReducer(config).reduce(UsageReported{Steps: 1})
		if !errors.Is(err, errExecutorContract) {
			t.Fatalf("error = %v, want executor protocol violation", err)
		}
	})

	t.Run("usage regression", func(t *testing.T) {
		config := testReducerConfig()
		config.Metrics = transcript.RunMetrics{
			Usage: &transcript.Usage{ModelUsage: transcript.ModelUsage{InputTokens: 10}},
			Steps: 1,
		}
		_, err := newReducer(config).reduce(UsageReported{
			TokenUsage: accounting.TokenUsage{PromptTokens: 9},
			Steps:      2,
		})
		if !errors.Is(err, errExecutorContract) {
			t.Fatalf("error = %v, want executor protocol violation", err)
		}
	})

	t.Run("per-model mismatch", func(t *testing.T) {
		_, err := newReducer(testReducerConfig()).reduce(UsageReported{
			TokenUsage: accounting.TokenUsage{PromptTokens: 5},
			ByModel: []accounting.ModelUsage{{
				Model:      "model",
				TokenUsage: accounting.TokenUsage{PromptTokens: 4},
				Calls:      1,
			}},
			Steps: 1,
		})
		if !errors.Is(err, errExecutorContract) {
			t.Fatalf("error = %v, want executor protocol violation", err)
		}
	})
}

type unsupportedEngineEvent struct{ engineEventBase }

func mustOpen(t *testing.T, reducer *reducer) []reduction {
	t.Helper()
	batch, err := reducer.open()
	if err != nil {
		t.Fatalf("open reducer: %v", err)
	}
	return testReductions(batch)
}

func mustReduce(t *testing.T, reducer *reducer, event EngineEvent) []reduction {
	t.Helper()
	batch, err := reducer.reduce(event)
	if err != nil {
		t.Fatalf("reduce %T: %v", event, err)
	}
	return testReductions(batch)
}

func mustReduceBatch(t *testing.T, reducer *reducer, event EngineEvent) reductionBatch {
	t.Helper()
	batch, err := reducer.reduce(event)
	if err != nil {
		t.Fatalf("reduce %T: %v", event, err)
	}
	return batch
}

// testReductions keeps the event-focused reducer tests concise. Production
// code carries a park commit on reductionBatch rather than assigning it to an
// arbitrary event; the test projection places it alongside the first event
// only for older helpers that inspect durable item contents.
func testReductions(batch reductionBatch) []reduction {
	events := slices.Clone(batch.events)
	if batch.parkCommit != nil && len(events) > 0 {
		events[0].Commit = batch.parkCommit
	}
	return events
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
	if !ok || started.Run.ID != "run_1" || started.Run.SessionID != "ses_1" || started.Run.ModelSelection.Model() != "claude" {
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

func mustReducerSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

func TestReducerResolvesSpawningItemByExecutorCallIdentity(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	first := mustReduce(t, reducer, ToolCallStart{
		CallID: "canonical-1", SourceCallID: "provider-1", ToolName: "delegate_task",
		Arguments: `{"description":"delegate"}`, SafetyClass: tool.SafetyClassExec,
	})
	want := startedItemID(t, first)
	got, err := reducer.spawningItem("provider-1")
	if err != nil {
		t.Fatalf("spawningItem: %v", err)
	}
	if got.ID != want ||
		got.RunID != "run_1" ||
		got.Status != transcript.ItemRunning ||
		got.Kind != transcript.ToolCall ||
		got.Tool == nil ||
		got.Tool.Name != "delegate_task" ||
		got.SafetyClass != tool.SafetyClassExec {
		t.Fatalf("spawningItem = %+v, want the canonical running tool item %q", got, want)
	}

	mustReduce(t, reducer, ToolCallStart{
		CallID: "canonical-2", SourceCallID: "provider-1", ToolName: "delegate_task", Arguments: `{}`,
	})
	if _, err := reducer.spawningItem("provider-1"); err == nil ||
		!strings.Contains(err.Error(), "multiple open tool items") {
		t.Fatalf("ambiguous spawningItem error = %v", err)
	}
	if _, err := reducer.spawningItem("provider-missing"); err == nil ||
		!strings.Contains(err.Error(), "no open tool item") {
		t.Fatalf("missing spawningItem error = %v", err)
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
	mustReduce(t, reducer, ToolCallStart{CallID: "shell_1", ToolName: "shell", Arguments: `{"command":"echo hi","description":"Print hi"}`})
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

	mustReduce(t, reducer, ToolCallStart{CallID: "write_1", ToolName: "write", Arguments: `{"path":"src/a.go"}`})
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
	denied := completedItem(t, mustReduce(t, reducer, ToolCallEnd{
		CallID: "denied_1",
		Problem: &transcript.Problem{
			Kind: transcript.DeniedByUserProblem, Scope: transcript.ToolProblem,
		},
	}))
	if denied.Status != transcript.ItemIncomplete || denied.Error == nil || denied.Error.Kind != transcript.DeniedByUserProblem {
		t.Fatalf("denied item = %+v", denied)
	}
}

func TestReducerCommitsConcurrentToolCompletionsInModelOrder(t *testing.T) {
	config := testReducerConfig()
	now := config.CreatedAt
	config.Now = func() time.Time { return now }
	reducer := newReducer(config)
	for _, event := range []ToolCallStart{
		{CallID: "call-1", ToolName: "first", Arguments: `{"value":1}`},
		{CallID: "call-2", ToolName: "second", Arguments: `{"value":2}`},
		{CallID: "call-3", ToolName: "third", Arguments: `{"value":3}`},
	} {
		mustReduce(t, reducer, event)
	}

	now = now.Add(time.Second)
	thirdFinishedAt := now
	if reduced := mustReduce(t, reducer, ToolCallEnd{CallID: "call-3", Result: testToolResult(t, "three")}); len(reduced) != 0 {
		t.Fatalf("third completion escaped ordering barrier: %+v", reduced)
	}
	now = now.Add(time.Second)
	first := mustReduce(t, reducer, ToolCallEnd{CallID: "call-1", Result: testToolResult(t, "one")})
	if got := completedToolNames(first); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("first completion batch = %v, want [first]", got)
	}
	now = now.Add(time.Second)
	secondFinishedAt := now
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
	var terminalTools []transcript.Item
	for _, event := range remaining {
		if completed, ok := event.Event.(ItemCompleted); ok {
			terminalTools = append(terminalTools, completed.Item)
		}
	}
	if len(terminalTools) != 2 || terminalTools[0].FinishedAt != secondFinishedAt || terminalTools[1].FinishedAt != thirdFinishedAt {
		t.Fatalf("completion times = %+v, want second=%s third=%s", terminalTools, secondFinishedAt, thirdFinishedAt)
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
		Kind: execution.ApprovalInterrupt,
		Approval: &ApprovalPrompt{
			CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write", Risk: "medium",
		},
	}}})
	commit := parked[0].Commit
	if commit == nil || commit.Run == nil || len(commit.Items) != 2 {
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
	if got := commit.Run.Interrupts[0].ItemID; got != firstID {
		t.Fatalf("approval item ID = %q, want original %q", got, firstID)
	}
	if len(reducer.drained) != 0 {
		t.Fatalf("completed or active approval leaked into drained tools: %+v", reducer.drained)
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
	firstCommit := mustReduce(t, first, TurnInterrupted{Interrupts: []Interrupt{{
		Kind: execution.ApprovalInterrupt,
		Approval: &ApprovalPrompt{
			CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write", Risk: "medium",
		},
	}}})[0].Commit
	if firstCommit == nil || firstCommit.Run == nil ||
		firstCommit.Run.Interrupts[0].ItemID != firstID ||
		len(first.drained) != 1 ||
		first.drained[0].ItemID != secondID ||
		first.drained[0].CallID != "call-2" {
		t.Fatalf("first park identity state = commit:%+v drained:%+v", firstCommit, first.drained)
	}

	config := testReducerConfig()
	config.SegmentID = "seg_2"
	config.Continuation = testTreeContinuation(interrupts.Pending{
		RootRunID:  "run_1",
		Interrupts: firstCommit.Run.Interrupts,
		Continuations: []interrupts.Continuation{{
			RunID: "run_1", DrainedTools: slices.Clone(first.drained),
		}},
	})
	resumed := newReducer(config)
	mustOpen(t, resumed)
	if got := startedItemID(t, mustReduce(t, resumed, ToolCallStart{
		CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
	})); got != firstID {
		t.Fatalf("resumed first item ID = %q, want %q", got, firstID)
	}
	mustReduce(t, resumed, ToolCallEnd{CallID: "call-1", Result: testToolResult(t, "approved")})

	secondCommit := mustReduce(t, resumed, TurnInterrupted{Interrupts: []Interrupt{{
		Kind: execution.ApprovalInterrupt,
		Approval: &ApprovalPrompt{
			CallID: "call-2", ToolName: "approval", Arguments: `{"path":"b"}`, SafetyClass: "write", Risk: "medium",
		},
	}}})[0].Commit
	if got := secondCommit.Run.Interrupts[0].ItemID; got != secondID {
		t.Fatalf("later approval item ID = %q, want original %q", got, secondID)
	}
	if len(resumed.drained) != 0 {
		t.Fatalf("surfaced later approval remained drained: %+v", resumed.drained)
	}
}

func TestReducerCanonicalProgressSnapshotsAndOutcomes(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	usage := mustReduce(t, reducer, UsageReported{
		TokenUsage: accounting.TokenUsage{PromptTokens: 1200, CompletionTokens: 80, ReasoningTokens: 30},
		CostUSD:    0.0125, Steps: 1, ContextTokens: 4096,
	})
	progress, ok := usage[0].Event.(SegmentProgressed)
	if !ok || progress.Progress.Usage == nil || progress.Progress.Usage.InputTokens != 1200 || progress.Progress.Usage.CostUSD == nil {
		t.Fatalf("usage progress = %#v", usage[0].Event)
	}
	if usage[0].Commit != nil {
		t.Fatal("usage progress must stay ephemeral")
	}

	snapshot := mustReduce(t, reducer, PlanUpdated{State: plan.State{
		Steps: []plan.Step{{
			Description: "write tests", Status: plan.StatusInProgress,
		}},
		Revision: 3, UpdatedAt: time.Unix(7, 0).UTC(),
	}})
	state, ok := snapshot[0].Event.(StateSnapshot)
	if !ok || len(state.Plan) != 1 || state.Plan[0].Description != "write tests" || state.Plan[0].Status != plan.StatusInProgress {
		t.Fatalf("plan snapshot = %#v", snapshot[0].Event)
	}
	if state.Revision != 3 || state.SessionID != "ses_1" {
		t.Fatalf("plan snapshot identity = %+v, want session ses_1 at revision 3", state)
	}

	compaction := mustReduce(t, reducer, CompactBoundary{MessagesBefore: 20, MessagesAfter: 6})
	if item := completedItem(t, compaction); item.Kind != transcript.Compaction || item.DroppedMessages != 14 {
		t.Fatalf("compaction item = %+v", item)
	}

	// The segment's last state snapshot is republished immediately before the
	// segment finishes, so whoever receives the finish has received the final value
	// — see reducer.fenceFinalState.
	terminal := mustReduce(t, reducer, TurnEnd{
		Reason: execution.OutcomeMaxBudget, Duration: 1500 * time.Millisecond,
		Usage: &TurnUsage{
			Tokens: accounting.TokenUsage{
				PromptTokens:     1200,
				CompletionTokens: 80,
				ReasoningTokens:  30,
			},
			CostUSD: 4.2,
			Steps:   1,
		},
	})
	finished := terminal[len(terminal)-1].Event.(SegmentFinished)
	if finished.Run.Metrics.ActiveDuration != 1500*time.Millisecond || finished.Run.Detail != "" {
		t.Fatalf("budget terminal = %+v", finished.Run)
	}
	fence, fenced := terminal[len(terminal)-2].Event.(StateSnapshot)
	if !fenced || fence.Revision != 3 {
		t.Fatalf("event before the finish = %#v, want the segment's final state snapshot", terminal[len(terminal)-2].Event)
	}
}

func TestReducerClassifiesErrorsWithoutLeakingProviderDetails(t *testing.T) {
	cases := []struct {
		name    string
		problem transcript.Problem
	}{
		{"rate limited", transcript.Problem{Kind: transcript.RateLimitedProblem, Detail: "retry shortly", RetryAfterSeconds: 30}},
		{"invalid credentials", transcript.Problem{Kind: transcript.InvalidAPIKeyProblem, Detail: "check credentials"}},
		{"provider unavailable", transcript.Problem{Kind: transcript.ProviderUnavailableProblem, Detail: "retry shortly"}},
		{"timeout", transcript.Problem{Kind: transcript.TimeoutProblem, Detail: "retry shortly"}},
		{"provider rejected", transcript.Problem{Kind: transcript.ProviderRejectedProblem, Detail: "invalid request"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reducer := newReducer(testReducerConfig())
			terminal := mustReduce(t, reducer, TurnEnd{Reason: execution.OutcomeError, Problem: &test.problem})
			finished := terminal[len(terminal)-1].Event.(SegmentFinished)
			problem := finished.Run.Error
			if problem == nil || *problem != (transcript.Problem{
				Kind: test.problem.Kind, Scope: transcript.RunProblem, Detail: test.problem.Detail,
				RetryAfterSeconds: test.problem.RetryAfterSeconds,
			}) || strings.Contains(problem.Detail, "api.example") {
				t.Errorf("terminal problem = %+v", problem)
			}
		})
	}
}

func TestReducerRejectsIncoherentTerminalProblems(t *testing.T) {
	tests := []struct {
		name  string
		event TurnEnd
	}{
		{name: "error without problem", event: TurnEnd{Reason: execution.OutcomeError}},
		{name: "completed with problem", event: TurnEnd{Reason: execution.OutcomeCompleted, Problem: &transcript.Problem{Kind: transcript.InternalProblem}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newReducer(testReducerConfig()).reduce(test.event)
			if !errors.Is(err, errExecutorContract) {
				t.Fatalf("reduce error = %v, want executor protocol violation", err)
			}
		})
	}
}

func TestReducerResumeReusesInterruptedItems(t *testing.T) {
	question := &transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Continue?"}}}
	approvalAt := time.Unix(1, 0).UTC()
	questionAt := time.Unix(2, 0).UTC()
	config := testReducerConfig()
	config.Continuation = testTreeContinuation(interrupts.Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Interrupts: []transcript.Interrupt{
			{ItemID: "item_approval", ItemOccurredAt: approvalAt, RunID: "run_1", Kind: execution.ApprovalInterrupt, Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Arguments: testToolArguments(t, map[string]any{"command": "go test", "description": "Run tests"})}, Risk: "medium"}},
			{ItemID: "item_question", ItemOccurredAt: questionAt, RunID: "run_1", Kind: execution.QuestionInterrupt, Question: question},
		},
	})
	reducer := newReducer(config)
	opening := mustOpen(t, reducer)
	completed, ok := opening[len(opening)-1].Event.(ItemCompleted)
	if !ok || completed.Item.ID != "item_question" || completed.Item.Question != question ||
		!completed.Item.OccurredAt.Equal(questionAt) {
		t.Fatalf("resumed question completion = %#v", opening[len(opening)-1].Event)
	}

	started := mustReduce(t, reducer, ToolCallStart{CallID: "call_1", ToolName: "shell", Arguments: `{"command":"go test","description":"Run tests"}`})
	var startedItem transcript.Item
	for _, reduction := range started {
		if event, ok := reduction.Event.(ItemStarted); ok {
			startedItem = event.Item
		}
	}
	if startedItem.ID != "item_approval" || !startedItem.OccurredAt.Equal(approvalAt) {
		t.Fatalf("resumed tool item = %+v, want original identity and occurrence", startedItem)
	}
	mustReduce(t, reducer, ToolCallEnd{CallID: "call_1", Result: testToolResult(t, "ok")})

	second := mustReduce(t, reducer, ToolCallStart{CallID: "call_2", ToolName: "shell", Arguments: `{"command":"go vet","description":"Vet server packages"}`})
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
	batch := mustReduceBatch(t, reducer, TurnInterrupted{Interrupts: []Interrupt{
		{Kind: execution.ApprovalInterrupt, Approval: &ApprovalPrompt{
			ToolName: "shell", Arguments: `{}`, SafetyClass: "exec", Risk: "high",
		}},
		{Kind: execution.QuestionInterrupt, Question: &QuestionPrompt{
			ToolName: "ask_user", Arguments: `{"questions":[{"question":"Continue?"}]}`,
			Fields: []QuestionFieldSpec{{Prompt: "Continue?"}},
		}},
	}})

	if batch.parkCommit == nil {
		t.Fatal("park has no atomic commit boundary")
	}
	if len(batch.events) == 0 {
		t.Fatal("park has no events")
	}
	first, ok := batch.events[0].Event.(ItemStarted)
	if !ok || first.Item.SessionID != "ses_1" {
		t.Fatalf("first park event = %#v, want first persisted interrupt item", batch.events[0].Event)
	}
	commit := batch.parkCommit
	if commit == nil || len(commit.Items) != 2 || commit.Run == nil || commit.State != StateSuspend {
		t.Fatalf("park commit = %+v, want items + run + suspend", commit)
	}
	for _, item := range commit.Items {
		if item.SessionID != "ses_1" || item.RunID != "run_1" || item.Status != transcript.ItemRunning {
			t.Fatalf("persisted interrupt item = %+v", item)
		}
	}
	if terminal := batch.events[len(batch.events)-1]; terminal.Commit != nil {
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
		{name: "malformed interrupt", event: TurnInterrupted{Interrupts: []Interrupt{{Kind: execution.InterruptKind(9)}}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := newReducer(testReducerConfig()).reduce(test.event)
			if !errors.Is(err, errExecutorContract) {
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
		if !errors.Is(err, errExecutorContract) || !errors.Is(err, tool.ErrInvalidArguments) {
			t.Fatalf("tool start error = %v, want executor protocol + invalid arguments", err)
		}
		if len(reducer.tools) != 0 || reducer.step != 0 {
			t.Fatalf("malformed start mutated reducer: tools=%d step=%d", len(reducer.tools), reducer.step)
		}
	})

	t.Run("effective end arguments", func(t *testing.T) {
		reducer := newReducer(testReducerConfig())
		mustReduce(t, reducer, ToolCallStart{
			CallID: "call_1", ToolName: "shell", Arguments: `{"command":"go test","description":"Run tests"}`,
		})
		_, err := reducer.reduce(ToolCallEnd{CallID: "call_1", Arguments: "null"})
		if !errors.Is(err, errExecutorContract) || !errors.Is(err, tool.ErrInvalidArguments) {
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
			if !errors.Is(err, errExecutorContract) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("reduce error = %v, want executor protocol containing %q", err, test.want)
			}
		})
	}

	t.Run("duplicate start", func(t *testing.T) {
		reducer := newReducer(testReducerConfig())
		start := ToolCallStart{CallID: "call_1", ToolName: "shell", Arguments: `{}`}
		mustReduce(t, reducer, start)
		_, err := reducer.reduce(start)
		if !errors.Is(err, errExecutorContract) || !strings.Contains(err.Error(), "started more than once") {
			t.Fatalf("duplicate start error = %v", err)
		}
	})
}

func TestReducerUsesCanonicalArgumentsForResumeIdentity(t *testing.T) {
	itemOccurredAt := time.Unix(1, 0).UTC()
	config := testReducerConfig()
	config.Continuation = testTreeContinuation(interrupts.Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []interrupts.Continuation{{
			RunID: "run_1",
			DrainedTools: []interrupts.DrainedTool{{
				ItemID: "item_original", ItemOccurredAt: itemOccurredAt, CallID: "old_call", Name: "lookup",
				Arguments: `{"b":2,"a":{"enabled":true}}`,
			}},
		}},
	})
	reducer := newReducer(config)
	started := mustReduce(t, reducer, ToolCallStart{
		CallID: "new_call", ToolName: "lookup",
		Arguments: "{\n  \"a\": {\"enabled\": true}, \"b\": 2\n}",
	})
	if got := startedItemID(t, started); got != "item_original" {
		t.Fatalf("resumed item id = %q, want canonical match item_original", got)
	}
	for _, reduction := range started {
		if event, ok := reduction.Event.(ItemStarted); ok && !event.Item.OccurredAt.Equal(itemOccurredAt) {
			t.Fatalf("resumed item occurrence = %s, want %s", event.Item.OccurredAt, itemOccurredAt)
		}
	}
}

func TestReducerRejectsMalformedDurableResumeArguments(t *testing.T) {
	config := testReducerConfig()
	config.Continuation = testTreeContinuation(interrupts.Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []interrupts.Continuation{{
			RunID: "run_1",
			DrainedTools: []interrupts.DrainedTool{{
				ItemID: "item_broken", ItemOccurredAt: time.Unix(1, 0).UTC(), Name: "lookup", Arguments: "[]",
			}},
		}},
	})
	_, err := newReducer(config).open()
	if !errors.Is(err, errReducerInvariant) || !errors.Is(err, tool.ErrInvalidArguments) {
		t.Fatalf("open error = %v, want reducer invariant + invalid arguments", err)
	}
}

func TestReducerConsumesHostCommittedToolResultWithoutDuplicatingTranscriptItem(t *testing.T) {
	config := testReducerConfig()
	config.Continuation = testTreeContinuation(interrupts.Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []interrupts.Continuation{{
			RunID: "run_1",
			CommittedTools: []interrupts.CommittedTool{{
				ItemID: "item_child", CallID: "call_child", Name: "delegate_task", Arguments: "{}",
				Problem: transcript.Problem{
					Kind:   transcript.ChildRunCanceledProblem,
					Scope:  transcript.ToolProblem,
					Detail: "stop delegated branch",
				},
			}},
		}},
	})
	reducer := newReducer(config)
	if _, err := reducer.open(); err != nil {
		t.Fatalf("open: %v", err)
	}

	batch, err := reducer.reduce(ToolCallEnd{
		CallID:    "call_child",
		Arguments: "{}",
		Problem: &transcript.Problem{
			Kind:   transcript.ToolFailedProblem,
			Scope:  transcript.ToolProblem,
			Detail: "delegated child canceled",
		},
	})
	if err != nil {
		t.Fatalf("consume committed ToolCallEnd: %v", err)
	}
	if len(batch.events) != 0 || batch.parkCommit != nil {
		t.Fatalf("committed tool result projected duplicate events: %+v", batch)
	}
	if remaining := reducer.resume.remainingCommittedTools(); len(remaining) != 0 {
		t.Fatalf("remaining committed tools = %+v, want none", remaining)
	}
}

func TestReducerRejectsReexecutionOrSuccessForHostCommittedTool(t *testing.T) {
	continuation := testTreeContinuation(interrupts.Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []interrupts.Continuation{{
			RunID: "run_1",
			CommittedTools: []interrupts.CommittedTool{{
				ItemID: "item_child", CallID: "call_child", Name: "delegate_task", Arguments: "{}",
				Problem: transcript.Problem{
					Kind:  transcript.ChildRunCanceledProblem,
					Scope: transcript.ToolProblem,
				},
			}},
		}},
	})
	t.Run("reexecution", func(t *testing.T) {
		config := testReducerConfig()
		config.Continuation = continuation
		reducer := newReducer(config)
		_, err := reducer.reduce(ToolCallStart{
			CallID: "call_child", ToolName: "delegate_task", Arguments: "{}",
		})
		if !errors.Is(err, errExecutorContract) ||
			!strings.Contains(err.Error(), "executed again") {
			t.Fatalf("ToolCallStart error = %v, want committed-call reexecution violation", err)
		}
	})
	t.Run("successful result", func(t *testing.T) {
		config := testReducerConfig()
		config.Continuation = continuation
		reducer := newReducer(config)
		_, err := reducer.reduce(ToolCallEnd{CallID: "call_child", Arguments: "{}"})
		if !errors.Is(err, errExecutorContract) ||
			!strings.Contains(err.Error(), "successful result") {
			t.Fatalf("ToolCallEnd error = %v, want committed-call success violation", err)
		}
	})
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
	parkCommit := func() *EventCommit {
		run := transcript.Run{State: execution.Interrupted}
		return &EventCommit{
			State: StateSuspend,
			Run:   &run,
		}
	}
	terminalCommit := func() *EventCommit {
		outcome := execution.OutcomeCompleted
		run := transcript.Run{State: execution.Completed, Outcome: &outcome}
		return &EventCommit{State: StateTerminalize, Outcome: outcome, Run: &run}
	}
	invalidTerminalCommit := terminalCommit()
	invalidTerminalCommit.Run.State = execution.Failed
	tests := []struct {
		name  string
		batch reductionBatch
	}{
		{name: "missing event", batch: reductionBatch{events: []reduction{{}}}},
		{name: "terminal is not last", batch: reductionBatch{events: []reduction{
			{Event: SegmentFinished{}, Commit: terminalCommit()},
			{Event: SegmentProgressed{}},
		}}},
		{name: "terminal has no commit", batch: reductionBatch{events: []reduction{{Event: SegmentFinished{}}}}},
		{name: "terminal lifecycle is inconsistent", batch: reductionBatch{events: []reduction{{Event: SegmentFinished{}, Commit: invalidTerminalCommit}}}},
		{name: "commit state is unknown", batch: reductionBatch{events: []reduction{{
			Event: ItemCompleted{}, Commit: &EventCommit{State: StateChange(255)},
		}}}},
		{name: "park has no terminal event", batch: reductionBatch{
			events: []reduction{{Event: ItemStarted{}}}, parkCommit: parkCommit(),
		}},
		{name: "park event repeats a durable commit", batch: reductionBatch{
			events: []reduction{
				{Event: ItemStarted{}, Commit: new(EventCommit)},
				{Event: SegmentFinished{}},
			},
			parkCommit: parkCommit(),
		}},
		{name: "park commit does not suspend", batch: reductionBatch{
			events: []reduction{{Event: SegmentFinished{}}}, parkCommit: new(EventCommit),
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateReductionBatch(test.batch); !errors.Is(err, errReducerInvariant) {
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

// TestReducerReportsFrozenRunCapabilitiesOnEverySegment pins what a resumed
// segment reports about the Run: the capabilities admitted with it, taken from
// the park's hand-off rather than an empty fresh-segment value or the resuming
// request.
//
// The opening event is checked as well as the park, because segment.started is
// where a reconnecting client learns what the Run may publish — a continuation
// announcing a minimal capability set would tell it to expect fewer frames than the Run
// can produce.
func TestReducerReportsFrozenRunCapabilitiesOnEverySegment(t *testing.T) {
	frozen := execution.RunCapabilities{
		ChildRuns:      true,
		InterruptKinds: []execution.InterruptKind{execution.ApprovalInterrupt},
	}
	config := testReducerConfig()
	config.Capabilities = frozen
	config.Continuation = testTreeContinuation(interrupts.Pending{
		RootRunID: "run_1", SessionID: "ses_1", Capabilities: frozen,
	})

	reducer := newReducer(config)
	opening := mustOpen(t, reducer)
	started, ok := opening[0].Event.(SegmentStarted)
	if !ok {
		t.Fatalf("opening event = %#v, want SegmentStarted", opening[0].Event)
	}
	assertFrozenCapabilities(t, started.Run.Capabilities, frozen, "segment.started")

	batch := mustReduceBatch(t, reducer, TurnInterrupted{Interrupts: []Interrupt{
		{Kind: execution.ApprovalInterrupt, Approval: &ApprovalPrompt{
			ToolName: "shell", Arguments: `{}`, SafetyClass: "exec", Risk: "high",
		}},
	}})
	if batch.parkCommit.Run == nil {
		t.Fatal("park commit carries no run record")
	}
	assertFrozenCapabilities(t, batch.parkCommit.Run.Capabilities, frozen, "parked run record")
}

func assertFrozenCapabilities(t *testing.T, got, want execution.RunCapabilities, where string) {
	t.Helper()
	if got.ChildRuns != want.ChildRuns || !slices.Equal(got.InterruptKinds, want.InterruptKinds) {
		t.Fatalf("%s capabilities = %v, want %v", where, got, want)
	}
}

// TestSegmentFencesItsFinalStateBeforeFinishing proves
// segment_fences_its_final_state: the last replayable event before a segment's finish
// is the final value of every state key that segment changed.
//
// The guarantee is POSITIONAL, and that is the point: a subscriber that attached
// late, or replayed from a cursor past the change itself, would otherwise reach
// segment.finished having never seen a snapshot and render a stale panel until
// something made it refetch. Whoever receives the finish has received the final
// value because the value is the event immediately before it.
//
// The second half matters as much: a segment that changed nothing publishes NO
// fence. An empty snapshot at revision 0 does not read as "unchanged" to a client
// that folds by revision — it reads as "the list was cleared".
func TestSegmentFencesItsFinalStateBeforeFinishing(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, PlanUpdated{State: plan.State{
		Steps:    []plan.Step{{Description: "fence this", Status: plan.StatusInProgress}},
		Revision: 4, UpdatedAt: time.Unix(11, 0).UTC(),
	}})

	terminal := mustReduce(t, reducer, TurnEnd{Reason: execution.OutcomeCompleted})
	if len(terminal) < 2 {
		t.Fatalf("terminal batch = %d events, want the fence and the finish", len(terminal))
	}
	if _, finished := terminal[len(terminal)-1].Event.(SegmentFinished); !finished {
		t.Fatalf("last event = %#v, want the segment finish", terminal[len(terminal)-1].Event)
	}
	fence, fenced := terminal[len(terminal)-2].Event.(StateSnapshot)
	if !fenced {
		t.Fatalf("event before the finish = %#v, want the segment's final state", terminal[len(terminal)-2].Event)
	}
	if fence.Revision != 4 || len(fence.Plan) != 1 || fence.SessionID != "ses_1" {
		t.Fatalf("fence = %+v, want session ses_1's revision 4 list", fence)
	}

	untouched := newReducer(testReducerConfig())
	quiet := mustReduce(t, untouched, TurnEnd{Reason: execution.OutcomeCompleted})
	for _, reduced := range quiet {
		if snapshot, ok := reduced.Event.(StateSnapshot); ok {
			t.Fatalf("a segment that changed no state published %+v", snapshot)
		}
	}
}
