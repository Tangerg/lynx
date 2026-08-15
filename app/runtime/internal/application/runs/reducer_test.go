package runs

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/conversation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	runfixture "github.com/Tangerg/lynx/app/runtime/internal/testsupport/runfixture"
	corechat "github.com/Tangerg/lynx/core/chat"
)

func testReducerConfig() reducerConfig {
	now := time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC)
	return reducerConfig{
		RunID: "run_1", SegmentID: "seg_1", SessionID: "ses_1", CWD: "/work",
		ExecutorID: "turn_1", ModelSelection: mustReducerSelection("anthropic", "claude"), CreatedAt: now,
		Now: func() time.Time { return now },
	}
}

func TestReducerTerminalIncludesGoalRunRecord(t *testing.T) {
	config := testReducerConfig()
	config.GoalIncarnationID = "goal_lease"
	reducer := newReducer(config)
	mustReduce(t, reducer, ToolCallStarted{CallID: "call_1", ToolName: "inspect", Arguments: `{}`})
	mustReduce(t, reducer, ToolCallFinished{CallID: "call_1", Result: testToolResult(t, "ok")})
	reductions := mustReduce(t, reducer, SegmentEnded{
		Reason: run.OutcomeCompleted,
		Usage:  &SegmentUsage{CostUSD: 0.75, Steps: 1},
	})
	commit := reductions[len(reductions)-1].Commit
	if commit == nil || commit.GoalRun == nil {
		t.Fatal("terminal commit did not carry Goal Run accounting")
	}
	if commit.TerminalCommitID == "" {
		t.Fatal("terminal commit did not carry an immutable write-set identity")
	}
	combined, err := combineTerminalEventCommit(reductionBatch{events: reductions})
	if err != nil {
		t.Fatalf("combine terminal commit: %v", err)
	}
	if combined.TerminalCommitID != commit.TerminalCommitID {
		t.Fatalf("combined terminal identity = %q, want %q", combined.TerminalCommitID, commit.TerminalCommitID)
	}
	want := goal.RunRecord{SessionID: "ses_1", IncarnationID: "goal_lease", RunID: "run_1", Outcome: run.OutcomeCompleted, CostUSD: 0.75, Steps: 1, CompletedAt: config.Now()}
	if got := *commit.GoalRun; got != want {
		t.Fatalf("GoalRun = %+v", got)
	}
}

func TestReducerStepsCountModelCallsRatherThanParallelTools(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, UsageReported{Steps: 1})
	mustReduce(t, reducer, ToolCallStarted{CallID: "call_1", ToolName: "inspect", Arguments: `{}`})
	mustReduce(t, reducer, ToolCallStarted{CallID: "call_2", ToolName: "inspect", Arguments: `{}`})
	mustReduce(t, reducer, ToolCallFinished{CallID: "call_2", Result: testToolResult(t, "two")})
	mustReduce(t, reducer, ToolCallFinished{CallID: "call_1", Result: testToolResult(t, "one")})
	finished := mustReduce(t, reducer, SegmentEnded{
		Reason: run.OutcomeCompleted,
		Usage:  &SegmentUsage{Steps: 1},
	})
	record := finished[len(finished)-1].Event.(SegmentFinished).Run
	if record.Metrics().Steps() != 1 {
		t.Fatalf("steps = %d, want one model call for two parallel tools", record.Metrics().Steps())
	}
}

func TestReducerEarlyExecutorFinalWaitsForAuthoritativeModelResponse(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})
	mustReduce(t, reducer, MessageDelta{Text: "partial"})
	message := corechat.NewAssistantMessage(
		corechat.NewReasoningPart("authoritative reasoning", nil),
		corechat.NewTextPart("authoritative answer"),
	)
	if reduced := mustReduce(t, reducer, AssistantMessageCompleted{Message: message}); len(reduced) != 0 {
		t.Fatalf("early executor confirmation projected transcript content: %#v", reduced)
	}
	reduced := mustReduce(t, reducer, ModelCallCompleted{
		CallID: "model_call_1", Message: message, Steps: 1,
	})

	var completed []transcript.Item
	for _, reduction := range reduced {
		if event, ok := reduction.Event.(ItemCompleted); ok {
			completed = append(completed, event.Item)
		}
	}
	if len(completed) != 2 {
		t.Fatalf("completed items = %#v", completed)
	}
	if completed[0].Kind() != transcript.Reasoning || completed[0].Text() != "authoritative reasoning" {
		t.Fatalf("reasoning completion = %#v", completed[0])
	}
	if completed[1].Kind() != transcript.AgentMessage || len(completed[1].Content()) != 1 ||
		completed[1].Content()[0].Text != "authoritative answer" {
		t.Fatalf("assistant completion = %#v", completed[1])
	}
}

func TestReducerKeepsStreamingItemsOpenUntilTheAuthoritativeModelResponse(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})
	reasoningBatch := mustReduce(t, reducer, ReasoningDelta{Text: "partial reasoning"})
	textBatch := mustReduce(t, reducer, MessageDelta{Text: "partial answer"})

	reasoningItemID := startedItemID(t, reasoningBatch)
	textItemID := startedItemID(t, textBatch)
	for _, batch := range [][]reduction{reasoningBatch, textBatch} {
		for _, reduced := range batch {
			if _, completed := reduced.Event.(ItemCompleted); completed {
				t.Fatalf("stream transition completed an Item before the model boundary: %#v", reduced.Event)
			}
		}
	}

	message := corechat.NewAssistantMessage(
		corechat.NewReasoningPart("authoritative reasoning", nil),
		corechat.NewTextPart("authoritative answer"),
	)
	completed := mustReduce(t, reducer, ModelCallCompleted{
		CallID: "model_call_1", Message: message, Steps: 1,
	})
	items := completedItems(completed)
	if len(items) != 2 {
		t.Fatalf("completed Items = %#v, want one reasoning and one assistant message", items)
	}
	if items[0].ID() != reasoningItemID || items[0].Kind() != transcript.Reasoning ||
		items[0].Text() != "authoritative reasoning" {
		t.Fatalf("reasoning completion = %#v, want authoritative completion of %q", items[0], reasoningItemID)
	}
	content := items[1].Content()
	if items[1].ID() != textItemID || items[1].Kind() != transcript.AgentMessage ||
		len(content) != 1 || content[0].Text != "authoritative answer" {
		t.Fatalf("assistant completion = %#v, want authoritative completion of %q", items[1], textItemID)
	}
}

func TestReducerDoesNotDuplicateModelFinalWhenExecutorConfirmsSameMessage(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	message := corechat.NewAssistantMessage(corechat.NewTextPart("authoritative answer"))
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})
	modelBatch := mustReduce(t, reducer, ModelCallCompleted{
		CallID: "model_call_1", Message: message, Steps: 1,
	})
	completed := 0
	for _, reduction := range modelBatch {
		if _, ok := reduction.Event.(ItemCompleted); ok {
			completed++
		}
	}
	if completed != 1 {
		t.Fatalf("model completion Item count = %d, want 1", completed)
	}
	conversation := committedConversationMessages(modelBatch)
	if len(conversation) != 1 || conversation[0].Text() != "authoritative answer" {
		t.Fatalf("model conversation projection = %#v", conversation)
	}
	processBatch := mustReduce(t, reducer, AssistantMessageCompleted{Message: message})
	if len(processBatch) != 0 {
		t.Fatalf("executor confirmation duplicated model final: %#v", processBatch)
	}
}

func TestReducerIgnoresStreamingObservationAfterAuthoritativeModelCompletion(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	message := corechat.NewAssistantMessage(corechat.NewTextPart("authoritative answer"))
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})
	mustReduce(t, reducer, MessageDelta{Text: "authoritative answer"})
	modelBatch := mustReduce(t, reducer, ModelCallCompleted{
		CallID: "model_call_1", Message: message, Steps: 1,
	})
	if completed := completedItems(modelBatch); len(completed) != 1 {
		t.Fatalf("model completion Items = %#v, want one", completed)
	}
	if late := mustReduce(t, reducer, MessageDelta{Text: "authoritative answer"}); len(late) != 0 {
		t.Fatalf("late stream observation reopened a transcript Item: %#v", late)
	}
	if confirmation := mustReduce(t, reducer, AssistantMessageCompleted{Message: message}); len(confirmation) != 0 {
		t.Fatalf("executor confirmation duplicated the model final: %#v", confirmation)
	}
}

func TestReducerProjectsAppliedSteersAsOneOrderedFact(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	reduced := mustReduce(t, reducer, SteerMessagesApplied{Messages: [][]transcript.ContentBlock{
		{{Kind: transcript.TextContent, Text: "first"}},
		{{Kind: transcript.TextContent, Text: "second"}},
	}})
	if len(reduced) != 2 {
		t.Fatalf("reductions = %d, want two complete Items", len(reduced))
	}
	var completed []transcript.Item
	for _, reduction := range reduced {
		if event, ok := reduction.Event.(ItemCompleted); ok {
			completed = append(completed, event.Item)
		}
	}
	if len(completed) != 2 || completed[0].Content()[0].Text != "first" ||
		completed[1].Content()[0].Text != "second" || completed[0].ID() == completed[1].ID() {
		t.Fatalf("completed steer items = %#v", completed)
	}
	conversation := committedConversationMessages(reduced)
	if len(conversation) != 2 || conversation[0].Text() != "first" || conversation[1].Text() != "second" {
		t.Fatalf("steer conversation projection = %#v", conversation)
	}
}

func TestReducerProjectsModelToolContextWithProviderCallIdentity(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	call := corechat.ToolCall{ID: "provider_call_1", Name: "inspect", Arguments: `{"path":"README.md"}`}
	message := corechat.NewAssistantMessage(corechat.NewToolCallPart(call))
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})
	modelBatch := mustReduce(t, reducer, ModelCallCompleted{
		CallID: "model_call_1", Message: message, Steps: 1,
	})
	modelMessages := committedConversationMessages(modelBatch)
	if len(modelMessages) != 1 || len(modelMessages[0].Parts) != 1 ||
		modelMessages[0].Parts[0].ToolCall == nil || modelMessages[0].Parts[0].ToolCall.ID != call.ID {
		t.Fatalf("model ToolCall conversation projection = %#v", modelMessages)
	}

	mustReduce(t, reducer, ToolCallStarted{
		CallID: "runtime_call_1", SourceCallID: call.ID,
		ModelCallSequence: 1, ToolCallIndex: 0,
		ToolName: call.Name, Arguments: call.Arguments, SafetyClass: tool.SafetyClassSafe,
	})
	toolBatch := mustReduce(t, reducer, ToolCallFinished{
		CallID: "runtime_call_1", Result: testToolResult(t, "contents"),
	})
	toolMessages := committedConversationMessages(toolBatch)
	if len(toolMessages) != 1 || toolMessages[0].Role != corechat.RoleTool || len(toolMessages[0].Parts) != 1 {
		t.Fatalf("Tool result conversation projection = %#v", toolMessages)
	}
	result := toolMessages[0].Parts[0].ToolResult
	if result == nil || result.ID != call.ID || result.Name != call.Name || result.Result != "contents" || result.IsError {
		t.Fatalf("Tool result = %#v, want provider identity and successful output", result)
	}
}

func TestReducerTerminalClosesProviderToolCallCanceledBeforeRuntimeStart(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	first := corechat.ToolCall{ID: "provider_first", Name: "glob", Arguments: `{"pattern":"**/*"}`}
	second := corechat.ToolCall{ID: "provider_second", Name: "search_tools", Arguments: `{"query":"lookup"}`}
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})
	modelBatch := mustReduce(t, reducer, ModelCallCompleted{
		CallID: "model_call_1",
		Message: corechat.NewAssistantMessage(
			corechat.NewToolCallPart(first),
			corechat.NewToolCallPart(second),
		),
		Steps: 1,
	})

	mustReduce(t, reducer, ToolCallStarted{
		CallID: "runtime_first", SourceCallID: first.ID,
		ModelCallSequence: 1, ToolCallIndex: 0,
		ToolName: first.Name, Arguments: first.Arguments,
	})
	toolBatch := mustReduce(t, reducer, ToolCallFinished{
		CallID: "runtime_first", Result: testToolResult(t, "first result"),
	})
	terminalBatch := mustReduce(t, reducer, SegmentEnded{Reason: run.OutcomeCanceled})
	terminalMessages := committedConversationMessages(terminalBatch)
	if len(terminalMessages) != 1 || terminalMessages[0].Role != corechat.RoleTool ||
		len(terminalMessages[0].Parts) != 1 {
		t.Fatalf("terminal conversation closure = %#v, want one Tool result", terminalMessages)
	}
	result := terminalMessages[0].Parts[0].ToolResult
	if result == nil || result.ID != second.ID || result.Name != second.Name || !result.IsError ||
		!strings.Contains(result.Result, "canceled") {
		t.Fatalf("terminal Tool result = %#v, want canceled %q", result, second.ID)
	}

	allMessages := append(committedConversationMessages(modelBatch), committedConversationMessages(toolBatch)...)
	allMessages = append(allMessages, terminalMessages...)
	history, err := conversation.New(allMessages)
	if err != nil {
		t.Fatalf("terminal conversation: %v", err)
	}
	_, stillOpen, err := history.CloseOpenToolCalls("still open")
	if err != nil || len(stillOpen) != 0 {
		t.Fatalf("terminal conversation left open Tool calls: messages=%#v err=%v", stillOpen, err)
	}
}

func TestReducerTerminalPreservesCompletedOutOfOrderToolResult(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	calls := []corechat.ToolCall{
		{ID: "provider_first", Name: "first", Arguments: `{}`},
		{ID: "provider_second", Name: "second", Arguments: `{}`},
	}
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})
	mustReduce(t, reducer, ModelCallCompleted{
		CallID: "model_call_1",
		Message: corechat.NewAssistantMessage(
			corechat.NewToolCallPart(calls[0]),
			corechat.NewToolCallPart(calls[1]),
		),
		Steps: 1,
	})
	for index, call := range calls {
		mustReduce(t, reducer, ToolCallStarted{
			CallID: "runtime_" + call.Name, SourceCallID: call.ID,
			ModelCallSequence: 1, ToolCallIndex: uint32(index),
			ToolName: call.Name, Arguments: call.Arguments,
		})
	}
	if messages := committedConversationMessages(mustReduce(t, reducer, ToolCallFinished{
		CallID: "runtime_second", Result: testToolResult(t, "known second"),
	})); len(messages) != 0 {
		t.Fatalf("out-of-order Tool result committed before its prefix: %#v", messages)
	}

	terminal := committedConversationMessages(mustReduce(t, reducer, SegmentEnded{Reason: run.OutcomeCanceled}))
	if len(terminal) != 1 || len(terminal[0].Parts) != 2 {
		t.Fatalf("terminal conversation closure = %#v, want two ordered results", terminal)
	}
	first := terminal[0].Parts[0].ToolResult
	second := terminal[0].Parts[1].ToolResult
	if first == nil || first.ID != calls[0].ID || !first.IsError ||
		second == nil || second.ID != calls[1].ID || second.IsError || second.Result != "known second" {
		t.Fatalf("terminal results = %#v, want canceled first then known second", terminal[0].Parts)
	}
}

func TestReducerRejectsMalformedAppliedSteerBatch(t *testing.T) {
	tests := map[string]SteerMessagesApplied{
		"empty batch":   {},
		"empty message": {Messages: [][]transcript.ContentBlock{{}}},
		"invalid content": {Messages: [][]transcript.ContentBlock{{{
			Kind: transcript.TextContent,
		}}}},
	}
	for name, fact := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := newReducer(testReducerConfig()).reduce(fact)
			if !errors.Is(err, errExecutorContract) {
				t.Fatalf("reduce error = %v, want executor contract violation", err)
			}
		})
	}
}

func TestReducerSynthesizesUnsettledModelCallAsAtomicRunLost(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, ModelCallStarted{CallID: "model_call_1"})

	batch, err := reducer.synthesizeTerminal()
	if err != nil {
		t.Fatal(err)
	}
	reductions := testReductions(batch)
	finished, ok := reductions[len(reductions)-1].Event.(SegmentFinished)
	if !ok || !runHasOutcome(finished.Run, run.OutcomeLost) ||
		!runHasFailureKind(finished.Run, run.FailureLost) {
		t.Fatalf("terminal = %#v, want RunLost", reductions[len(reductions)-1].Event)
	}
	commit := reductions[len(reductions)-1].Commit
	if commit == nil || len(commit.ModelInvocations) != 1 ||
		commit.ModelInvocations[0].State != ModelInvocationUnknown {
		t.Fatalf("model invocation settlement = %#v, want one unknown", commit)
	}
}

func TestReducerTreatsExecutorAccountingAsCumulativeAcrossResume(t *testing.T) {
	config := testReducerConfig()
	config.Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Usage: &accounting.Usage{Total: accounting.Totals{
		InputTokens: 10,
	}},
		Steps:          2,
		ActiveDuration: time.Second})

	reducer := newReducer(config)
	mustReduce(t, reducer, UsageReported{
		TokenUsage: accounting.TokenUsage{PromptTokens: 15},
		Steps:      3,
	})
	finished := mustReduce(t, reducer, SegmentEnded{
		Reason: run.OutcomeCompleted,
		Usage: &SegmentUsage{
			Tokens: accounting.TokenUsage{PromptTokens: 15},
			Steps:  3,
		},
		Duration: 2 * time.Second,
	})
	record := finished[len(finished)-1].Event.(SegmentFinished).Run
	metrics := record.Metrics()
	usage, reported := metrics.Usage()
	if metrics.Steps() != 3 || !reported ||
		usage.Total.InputTokens != 15 ||
		metrics.ActiveDuration() != 3*time.Second {
		t.Fatalf("cumulative metrics = %+v", metrics)
	}
}

func TestReducerRejectsInconsistentOrRegressingAccounting(t *testing.T) {
	t.Run("step regression", func(t *testing.T) {
		config := testReducerConfig()
		config.Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Steps: 2})
		_, err := newReducer(config).reduce(UsageReported{Steps: 1})
		if !errors.Is(err, errExecutorContract) {
			t.Fatalf("error = %v, want executor protocol violation", err)
		}
	})

	t.Run("usage regression", func(t *testing.T) {
		config := testReducerConfig()
		config.Metrics = runfixture.MustMetrics(runfixture.MetricsInput{Usage: &accounting.Usage{Total: accounting.Totals{InputTokens: 10}},
			Steps: 1})

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

type unsupportedEngineEvent struct{ executionFactBase }

func mustOpen(t *testing.T, reducer *reducer) []reduction {
	t.Helper()
	batch, err := reducer.open()
	if err != nil {
		t.Fatalf("open reducer: %v", err)
	}
	return testReductions(batch)
}

func mustReduce(t *testing.T, reducer *reducer, event ExecutionFact) []reduction {
	t.Helper()
	batch, err := reducer.reduce(event)
	if err != nil {
		t.Fatalf("reduce %T: %v", event, err)
	}
	return testReductions(batch)
}

func mustReduceBatch(t *testing.T, reducer *reducer, event ExecutionFact) reductionBatch {
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
	if len(opening) != 2 {
		t.Fatalf("opening reductions = %d, want segment + complete user Item", len(opening))
	}
	started, ok := opening[0].Event.(SegmentStarted)
	if !ok || started.Run.ID() != "run_1" || started.Run.SessionID() != "ses_1" || started.Run.ModelSelection().Model() != "claude" {
		t.Fatalf("opening run = %#v", opening[0].Event)
	}
	itemCompleted, ok := opening[1].Event.(ItemCompleted)
	if !ok || itemCompleted.Item.Kind() != transcript.UserMessage ||
		itemCompleted.Item.SessionID() != "ses_1" || itemCompleted.Item.Content()[0].Text != "hello" {
		t.Fatalf("user item completion = %#v", opening[1].Event)
	}
	if opening[1].Commit == nil || len(opening[1].Commit.Items) != 1 {
		t.Fatal("completed user item has no canonical durable fact")
	}
	conversation := opening[1].Commit.ConversationMessages
	if len(conversation) != 1 || conversation[0].Role != corechat.RoleUser || conversation[0].Text() != "hello" {
		t.Fatalf("opening conversation projection = %#v", conversation)
	}
	if again := mustOpen(t, reducer); len(again) != 1 {
		t.Fatalf("second opening repeated user input: %+v", again)
	}
}

func committedConversationMessages(reductions []reduction) []corechat.Message {
	var messages []corechat.Message
	for _, reduced := range reductions {
		if reduced.Commit != nil {
			messages = append(messages, reduced.Commit.ConversationMessages...)
		}
	}
	return messages
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
	first := mustReduce(t, reducer, ToolCallStarted{
		CallID: "canonical-1", SourceCallID: "provider-1", ToolName: "delegate_task",
		Arguments: `{"description":"delegate"}`, SafetyClass: tool.SafetyClassExec,
	})
	want := startedItemID(t, first)
	got, err := reducer.spawningItem("provider-1")
	if err != nil {
		t.Fatalf("spawningItem: %v", err)
	}
	invocation, present := got.ToolInvocation()
	if got.ID() != want ||
		got.RunID() != "run_1" ||
		got.Status() != transcript.ItemRunning ||
		got.Kind() != transcript.ToolCall ||
		!present || invocation.Name != "delegate_task" ||
		got.SafetyClass() != tool.SafetyClassExec {
		t.Fatalf("spawningItem = %+v, want the canonical running tool item %q", got, want)
	}

	mustReduce(t, reducer, ToolCallStarted{
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
	completed, ok := opening[1].Event.(ItemCompleted)
	if !ok || len(completed.Item.Content()) != 1 || completed.Item.Content()[0].Text != "original" {
		t.Fatalf("opening user item = %#v, want owned original input", opening[1].Event)
	}
}

func TestReducerPreservesRawToolResultsAndExplicitFileNudges(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	mustReduce(t, reducer, ToolCallStarted{CallID: "shell_1", ToolName: "shell", Arguments: `{"command":"echo hi","description":"Print hi"}`})
	raw := map[string]any{"stdout": "hi\n", "stderr": "oops", "exit_code": 0}
	reduced := mustReduce(t, reducer, ToolCallFinished{
		CallID: "shell_1", Result: testToolResult(t, raw), OutputText: "hi\n\noops",
	})
	completed := completedItem(t, reduced)
	invocation, present := completed.ToolInvocation()
	if !present {
		t.Fatal("completed tool is nil")
	}
	result, ok := invocation.Result.Any().(map[string]any)
	if !ok || result["stdout"] != "hi\n" || result["stderr"] != "oops" || result["exit_code"] != json.Number("0") {
		t.Fatalf("raw command result = %#v", invocation.Result)
	}

	mustReduce(t, reducer, ToolCallStarted{CallID: "write_1", ToolName: "write", Arguments: `{"path":"src/a.go"}`})
	write := mustReduce(t, reducer, ToolCallFinished{
		CallID: "write_1", Result: testToolResult(t, map[string]any{}), MutatedPaths: []string{"src/a.go"},
	})
	var nudge *Nudge
	for _, reduction := range write {
		if reduction.Nudge != nil {
			nudge = reduction.Nudge
		}
	}
	if nudge == nil || nudge.CWD != "/work" || len(nudge.Paths) != 1 || nudge.Paths[0] != "src/a.go" {
		t.Fatalf("write nudge = %+v", nudge)
	}

	mustReduce(t, reducer, ToolCallStarted{CallID: "partial_1", ToolName: "write", Arguments: `{"path":"src/partial.go"}`})
	partial := mustReduce(t, reducer, ToolCallFinished{
		CallID: "partial_1", MutatedPaths: []string{"src/partial.go"},
		Failure: &tool.Failure{Kind: tool.FailureExecution, Detail: "post-write failure"},
	})
	nudge = nil
	for _, reduction := range partial {
		if reduction.Nudge != nil {
			nudge = reduction.Nudge
		}
	}
	if nudge == nil || len(nudge.Paths) != 1 || nudge.Paths[0] != "src/partial.go" {
		t.Fatalf("partial write nudge = %+v", nudge)
	}

	mustReduce(t, reducer, ToolCallStarted{CallID: "denied_1", ToolName: "shell", Arguments: `{}`})
	denied := completedItem(t, mustReduce(t, reducer, ToolCallFinished{
		CallID:  "denied_1",
		Failure: &tool.Failure{Kind: tool.FailureDenied},
	}))
	failure, failed := denied.Failure()
	if denied.Status() != transcript.ItemIncomplete || !failed || failure.Kind != tool.FailureDenied {
		t.Fatalf("denied item = %+v", denied)
	}
}

func TestReducerCommitsConcurrentToolCompletionsInModelOrder(t *testing.T) {
	config := testReducerConfig()
	now := config.CreatedAt
	config.Now = func() time.Time { return now }
	reducer := newReducer(config)
	for _, event := range []ToolCallStarted{
		{CallID: "call-1", ToolName: "first", Arguments: `{"value":1}`},
		{CallID: "call-2", ToolName: "second", Arguments: `{"value":2}`},
		{CallID: "call-3", ToolName: "third", Arguments: `{"value":3}`},
	} {
		mustReduce(t, reducer, event)
	}

	now = now.Add(time.Second)
	thirdFinishedAt := now
	if reduced := mustReduce(t, reducer, ToolCallFinished{CallID: "call-3", Result: testToolResult(t, "three")}); len(reduced) != 0 {
		t.Fatalf("third completion escaped ordering barrier: %+v", reduced)
	}
	now = now.Add(time.Second)
	first := mustReduce(t, reducer, ToolCallFinished{CallID: "call-1", Result: testToolResult(t, "one")})
	if got := completedToolNames(first); !slices.Equal(got, []string{"first"}) {
		t.Fatalf("first completion batch = %v, want [first]", got)
	}
	now = now.Add(time.Second)
	secondFinishedAt := now
	remaining := mustReduce(t, reducer, ToolCallFinished{
		CallID: "call-2", Arguments: `{"value":20}`, Result: testToolResult(t, "two"),
	})
	if got := completedToolNames(remaining); !slices.Equal(got, []string{"second", "third"}) {
		t.Fatalf("released completion batch = %v, want [second third]", got)
	}
	second := completedItem(t, remaining)
	secondInvocation, _ := second.ToolInvocation()
	if secondInvocation.Arguments.Map()["value"] != json.Number("20") {
		t.Fatalf("effective arguments = %#v, want value 20", secondInvocation.Arguments)
	}
	var terminalTools []transcript.Item
	for _, event := range remaining {
		if completed, ok := event.Event.(ItemCompleted); ok {
			terminalTools = append(terminalTools, completed.Item)
		}
	}
	if len(terminalTools) != 2 || !terminalTools[0].FinishedAt().Equal(secondFinishedAt) || !terminalTools[1].FinishedAt().Equal(thirdFinishedAt) {
		t.Fatalf("completion times = %+v, want second=%s third=%s", terminalTools, secondFinishedAt, thirdFinishedAt)
	}
}

func TestReducerParksConcurrentToolsWithoutLosingCompletedResults(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	firstStart := mustReduce(t, reducer, ToolCallStarted{
		CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
	})
	firstID := startedItemID(t, firstStart)
	mustReduce(t, reducer, ToolCallStarted{
		CallID: "call-2", ToolName: "lookup", Arguments: `{"path":"b"}`, SafetyClass: tool.SafetyClassSafe,
	})
	if reduced := mustReduce(t, reducer, ToolCallFinished{CallID: "call-2", Result: testToolResult(t, "found")}); len(reduced) != 0 {
		t.Fatalf("later completion escaped paused prefix: %+v", reduced)
	}

	parked := mustReduce(t, reducer, SegmentInterrupted{Interrupts: []Interrupt{{
		Kind: interrupt.Approval,
		Approval: &ApprovalPrompt{
			CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write", Risk: "medium",
		},
	}}})
	commit := parked[0].Commit
	if commit == nil || commit.Run == nil || len(commit.Items) != 2 {
		t.Fatalf("park commit = %+v, want two ordered tool items", commit)
	}
	if commit.Items[0].ID() != firstID || commit.Items[0].Status() != transcript.ItemRunning {
		t.Fatalf("active approval item = %+v, want original running item %q", commit.Items[0], firstID)
	}
	siblingInvocation, present := commit.Items[1].ToolInvocation()
	if !present || siblingInvocation.Result == nil {
		t.Fatalf("completed sibling item = %+v", commit.Items[1])
	}
	result, ok := siblingInvocation.Result.String()
	if siblingInvocation.Name != "lookup" ||
		commit.Items[1].Status() != transcript.ItemCompleted || !ok || result != "found" {
		t.Fatalf("completed sibling item = %+v", commit.Items[1])
	}
	interrupted := parked[len(parked)-1].Event.(SegmentFinished)
	if got := interrupted.Interrupts[0].ItemID; got != firstID {
		t.Fatalf("approval item ID = %q, want original %q", got, firstID)
	}
	if len(reducer.drained) != 0 {
		t.Fatalf("completed or active approval leaked into drained tools: %+v", reducer.drained)
	}
}

func TestReducerCarriesLaterPausedCallIdentityAcrossSequentialResumes(t *testing.T) {
	first := newReducer(testReducerConfig())
	firstID := startedItemID(t, mustReduce(t, first, ToolCallStarted{
		CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
	}))
	secondID := startedItemID(t, mustReduce(t, first, ToolCallStarted{
		CallID: "call-2", ToolName: "approval", Arguments: `{"path":"b"}`, SafetyClass: "write",
	}))
	firstPark := mustReduce(t, first, SegmentInterrupted{Interrupts: []Interrupt{{
		Kind: interrupt.Approval,
		Approval: &ApprovalPrompt{
			CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write", Risk: "medium",
		},
	}}})
	firstCommit := firstPark[0].Commit
	firstInterrupted := firstPark[len(firstPark)-1].Event.(SegmentFinished)
	if firstCommit == nil || firstCommit.Run == nil ||
		firstInterrupted.Interrupts[0].ItemID != firstID ||
		len(first.drained) != 1 ||
		first.drained[0].ItemID != secondID ||
		first.drained[0].CallID != "call-2" {
		t.Fatalf("first park identity state = commit:%+v drained:%+v", firstCommit, first.drained)
	}

	config := testReducerConfig()
	config.SegmentID = "seg_2"
	config.Continuation = testTreeContinuation(Pending{
		RootRunID:  "run_1",
		Interrupts: firstInterrupted.Interrupts,
		Continuations: []Continuation{{
			RunID: "run_1", DrainedTools: slices.Clone(first.drained),
		}},
	})
	resumed := newReducer(config)
	mustOpen(t, resumed)
	resumedStart := mustReduce(t, resumed, ToolCallStarted{
		CallID: "call-1", ToolName: "approval", Arguments: `{"path":"a"}`, SafetyClass: "write",
	})
	for _, reduced := range resumedStart {
		if _, ok := reduced.Event.(ItemStarted); ok {
			t.Fatal("resumed canonical Tool Item published a second lifecycle start")
		}
	}
	if got, open := resumed.openToolItemID("call-1"); !open || got != firstID {
		t.Fatalf("resumed first item = %q/%t, want %q", got, open, firstID)
	}
	mustReduce(t, resumed, ToolCallFinished{CallID: "call-1", Result: testToolResult(t, "approved")})

	secondPark := mustReduce(t, resumed, SegmentInterrupted{Interrupts: []Interrupt{{
		Kind: interrupt.Approval,
		Approval: &ApprovalPrompt{
			CallID: "call-2", ToolName: "approval", Arguments: `{"path":"b"}`, SafetyClass: "write", Risk: "medium",
		},
	}}})
	secondInterrupted := secondPark[len(secondPark)-1].Event.(SegmentFinished)
	if got := secondInterrupted.Interrupts[0].ItemID; got != secondID {
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
	if !ok || progress.Progress.Usage == nil || progress.Progress.Usage.Total.InputTokens != 1200 || progress.Progress.Usage.Total.CostUSD == nil {
		t.Fatalf("usage progress = %#v", usage[0].Event)
	}
	if usage[0].Commit != nil {
		t.Fatal("usage progress must stay ephemeral")
	}

	snapshot := mustReduce(t, reducer, PlanUpdated{State: testPlanState(t, plan.Snapshot{
		Steps: []plan.Step{{
			Description: "write tests", Status: plan.StatusInProgress,
		}},
		Revision: 3, UpdatedAt: time.Unix(7, 0).UTC(),
	})})
	state, ok := snapshot[0].Event.(StateSnapshot)
	if !ok || len(state.Plan) != 1 || state.Plan[0].Description != "write tests" || state.Plan[0].Status != plan.StatusInProgress {
		t.Fatalf("plan snapshot = %#v", snapshot[0].Event)
	}
	if state.Revision != 3 || state.SessionID != "ses_1" {
		t.Fatalf("plan snapshot identity = %+v, want session ses_1 at revision 3", state)
	}

	compaction := mustReduce(t, reducer, CompactionBoundary{MessagesBefore: 20, MessagesAfter: 6})
	if item := completedItem(t, compaction); item.Kind() != transcript.Compaction || item.DroppedMessages() != 14 {
		t.Fatalf("compaction item = %+v", item)
	}

	// The segment's last state snapshot is republished immediately before the
	// segment finishes, so whoever receives the finish has received the final value
	// — see reducer.fenceFinalState.
	terminal := mustReduce(t, reducer, SegmentEnded{
		Reason: run.OutcomeMaxBudget, Duration: 1500 * time.Millisecond,
		Usage: &SegmentUsage{
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
	if finished.Run.Metrics().ActiveDuration() != 1500*time.Millisecond || finished.Run.Detail() != "" {
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
		failure run.Failure
	}{
		{"rate limited", run.Failure{Kind: run.FailureRateLimited, Detail: "retry shortly", RetryAfter: 30 * time.Second}},
		{"invalid credentials", run.Failure{Kind: run.FailureInvalidCredentials, Detail: "check credentials"}},
		{"provider unavailable", run.Failure{Kind: run.FailureProviderUnavailable, Detail: "retry shortly"}},
		{"timeout", run.Failure{Kind: run.FailureTimeout, Detail: "retry shortly"}},
		{"provider rejected", run.Failure{Kind: run.FailureProviderRejected, Detail: "invalid request"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			reducer := newReducer(testReducerConfig())
			terminal := mustReduce(t, reducer, SegmentEnded{Reason: run.OutcomeFailed, Failure: &test.failure})
			finished := terminal[len(terminal)-1].Event.(SegmentFinished)
			failure, failed := finished.Run.Failure()
			if !failed || failure != test.failure || strings.Contains(failure.Detail, "api.example") {
				t.Errorf("terminal failure = %+v", failure)
			}
		})
	}
}

func TestReducerRejectsIncoherentTerminalProblems(t *testing.T) {
	tests := []struct {
		name  string
		event SegmentEnded
	}{
		{name: "error without problem", event: SegmentEnded{Reason: run.OutcomeFailed}},
		{name: "completed with failure", event: SegmentEnded{Reason: run.OutcomeCompleted, Failure: &run.Failure{Kind: run.FailureInternal}}},
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
	config.Continuation = testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Interrupts: []transcript.Interrupt{
			{ItemID: "item_approval", ItemOccurredAt: approvalAt, RunID: "run_1", Kind: interrupt.Approval, Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Arguments: testToolArguments(t, map[string]any{"command": "go test", "description": "Run tests"})}, Risk: "medium"}},
			{ItemID: "item_question", ItemOccurredAt: questionAt, RunID: "run_1", Kind: interrupt.Question, Question: question},
		},
	})
	reducer := newReducer(config)
	opening := mustOpen(t, reducer)
	if len(opening) != 1 {
		t.Fatalf("resume opening = %#v, Question prompt must not complete twice", opening)
	}

	started := mustReduce(t, reducer, ToolCallStarted{CallID: "call_1", ToolName: "shell", Arguments: `{"command":"go test","description":"Run tests"}`})
	for _, reduction := range started {
		if _, ok := reduction.Event.(ItemStarted); ok {
			t.Fatal("resumed canonical Tool Item published a second lifecycle start")
		}
	}
	if itemID, open := reducer.openToolItemID("call_1"); !open || itemID != "item_approval" {
		t.Fatalf("resumed open tool = %q/%t, want item_approval", itemID, open)
	}
	completed := completedItem(t, mustReduce(t, reducer, ToolCallFinished{CallID: "call_1", Result: testToolResult(t, "ok")}))
	if completed.ID() != "item_approval" || !completed.OccurredAt().Equal(approvalAt) {
		t.Fatalf("resumed completed tool = %s/%s, want original identity and occurrence", completed.ID(), completed.OccurredAt())
	}

	second := mustReduce(t, reducer, ToolCallStarted{CallID: "call_2", ToolName: "shell", Arguments: `{"command":"go vet","description":"Vet server packages"}`})
	var secondID string
	for _, reduction := range second {
		if event, ok := reduction.Event.(ItemStarted); ok {
			secondID = event.Item.ItemID
		}
	}
	if secondID == "" || secondID == "item_approval" {
		t.Fatalf("new same-name tool item id = %q, want a fresh identity", secondID)
	}
}

func TestReducerProjectsParkAsOneAtomicWriteSetBeforeFirstInterruptEvent(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	batch := mustReduceBatch(t, reducer, SegmentInterrupted{Interrupts: []Interrupt{
		{Kind: interrupt.Approval, Approval: &ApprovalPrompt{
			ToolName: "shell", Arguments: `{}`, SafetyClass: "exec", Risk: "high",
		}},
		{Kind: interrupt.Question, Question: &QuestionPrompt{
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
		wantStatus := transcript.ItemRunning
		if item.Kind() == transcript.QuestionItem {
			wantStatus = transcript.ItemCompleted
		}
		if item.SessionID() != "ses_1" || item.RunID() != "run_1" || item.Status() != wantStatus {
			t.Fatalf("persisted interrupt item = %+v", item)
		}
	}
	if terminal := batch.events[len(batch.events)-1]; terminal.Commit != nil {
		t.Fatalf("terminal event repeated park commit: %+v", terminal)
	}
}

func TestReducerPublishesOneStartForToolThatBecomesAnApproval(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	started := mustReduce(t, reducer, ToolCallStarted{
		CallID: "call_1", SourceCallID: "provider_1", ModelCallSequence: 1,
		ToolName: "shell", Arguments: `{}`,
	})
	parked := mustReduceBatch(t, reducer, SegmentInterrupted{Interrupts: []Interrupt{{
		Kind: interrupt.Approval,
		Approval: &ApprovalPrompt{
			CallID: "call_1", ToolName: "shell", Arguments: `{}`,
			SafetyClass: tool.SafetyClassExec, Risk: "high",
		},
	}}})

	starts := 0
	for _, reduced := range append(started, parked.events...) {
		if _, ok := reduced.Event.(ItemStarted); ok {
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("tool start events = %d, want one canonical lifecycle start", starts)
	}
	if parked.parkCommit == nil || len(parked.parkCommit.Items) != 1 {
		t.Fatalf("park commit = %+v, want the running approval Item", parked.parkCommit)
	}
	if len(parked.parkCommit.ToolInvocations) != 1 ||
		parked.parkCommit.ToolInvocations[0].State != ToolInvocationIncomplete {
		t.Fatalf("park Tool invocations = %+v, want one incomplete attempt", parked.parkCommit.ToolInvocations)
	}
}

func TestReducerKeepsQuestionToolLifecycleOpenAcrossHITLResume(t *testing.T) {
	const (
		arguments    = `{"questions":[{"header":"Color","options":[{"label":"Blue"},{"label":"Green"}],"question":"Choose"}]}`
		providerArgs = `{"questions":[{"header":"Color","options":[{"description":"","label":"Blue"},{"description":"","label":"Green"}],"question":"Choose"}]}`
	)
	first := newReducer(testReducerConfig())
	toolItemID := startedItemID(t, mustReduce(t, first, ToolCallStarted{
		CallID: "call_question", SourceCallID: "provider_question",
		ModelCallSequence: 1, ToolName: "ask_user", Arguments: providerArgs,
		SafetyClass: tool.SafetyClassSafe,
	}))
	parked := mustReduceBatch(t, first, SegmentInterrupted{Interrupts: []Interrupt{{
		Kind: interrupt.Question,
		Question: &QuestionPrompt{
			CallID: "call_question", ToolName: "ask_user", Arguments: arguments,
			Fields: []QuestionFieldSpec{{
				Prompt: "Choose", Header: "Color",
				Options:     []QuestionOptionSpec{{Label: "Blue"}, {Label: "Green"}},
				AllowCustom: true,
			}},
		},
	}}})

	for _, reduced := range parked.events {
		completed, ok := reduced.Event.(ItemCompleted)
		if ok && completed.Item.ID() == toolItemID {
			t.Fatal("question park completed its still-suspended Tool Item")
		}
	}
	if parked.parkCommit == nil || len(parked.parkCommit.Items) != 2 {
		t.Fatalf("question park commit = %+v, want running Tool and completed Question", parked.parkCommit)
	}
	if parked.parkCommit.Items[0].ID() != toolItemID ||
		parked.parkCommit.Items[0].Status() != transcript.ItemRunning ||
		parked.parkCommit.Items[1].Kind() != transcript.QuestionItem ||
		parked.parkCommit.Items[1].Status() != transcript.ItemCompleted {
		t.Fatalf("question park Items = %+v", parked.parkCommit.Items)
	}
	if len(first.drained) != 1 || first.drained[0].ItemID != toolItemID {
		t.Fatalf("question continuation tools = %+v, want %q", first.drained, toolItemID)
	}
	if len(parked.parkCommit.ToolInvocations) != 1 ||
		parked.parkCommit.ToolInvocations[0].CallID != "call_question" ||
		parked.parkCommit.ToolInvocations[0].ItemID != toolItemID ||
		parked.parkCommit.ToolInvocations[0].State != ToolInvocationIncomplete {
		t.Fatalf("question park Tool invocations = %+v, want closed first-segment attempt", parked.parkCommit.ToolInvocations)
	}
	finished := parked.events[len(parked.events)-1].Event.(SegmentFinished)
	if len(finished.Interrupts) != 1 || finished.Interrupts[0].ItemID == toolItemID {
		t.Fatalf("question interrupt = %+v, want a separate prompt Item", finished.Interrupts)
	}

	config := testReducerConfig()
	config.SegmentID = "seg_2"
	resumeNow := config.CreatedAt.Add(time.Minute)
	config.Now = func() time.Time { return resumeNow }
	config.Continuation = testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Interrupts: finished.Interrupts,
		Continuations: []Continuation{{
			RunID: "run_1", DrainedTools: slices.Clone(first.drained),
		}},
	})
	resumed := newReducer(config)
	mustOpen(t, resumed)
	refired := mustReduce(t, resumed, ToolCallStarted{
		CallID: "call_question", SourceCallID: "provider_question",
		ModelCallSequence: 1, ToolName: "ask_user", Arguments: providerArgs,
		SafetyClass: tool.SafetyClassSafe,
	})
	startedAttempts := 0
	for _, reduced := range refired {
		if _, started := reduced.Event.(ItemStarted); started {
			t.Fatal("resumed question Tool published a second lifecycle start")
		}
		if reduced.Commit != nil && len(reduced.Commit.ToolInvocations) == 1 &&
			reduced.Commit.ToolInvocations[0].State == ToolInvocationStarted {
			startedAttempts++
		}
	}
	if startedAttempts != 1 {
		t.Fatalf("resumed Tool started attempts = %d, want one new segment attempt", startedAttempts)
	}
	resumeNow = resumeNow.Add(2 * time.Second)
	completed := completedItem(t, mustReduce(t, resumed, ToolCallFinished{
		CallID: "call_question", Result: testToolResult(t, "Blue"),
	}))
	if completed.ID() != toolItemID || completed.Status() != transcript.ItemCompleted {
		t.Fatalf("resumed question Tool = %+v, want one completion of %q", completed, toolItemID)
	}
	duration, known := completed.ExecutionDuration()
	if !known || duration != 2*time.Second {
		t.Fatalf("resumed Tool execution duration = %v/%t, want 2s excluding HITL wait", duration, known)
	}
	if lifecycle := completed.FinishedAt().Sub(completed.OccurredAt()); lifecycle <= duration {
		t.Fatalf("resumed Tool lifecycle = %v, want greater than execution duration %v", lifecycle, duration)
	}
}

func TestReducerRejectsExecutorProtocolViolations(t *testing.T) {
	tests := []struct {
		name  string
		event ExecutionFact
	}{
		{name: "unknown event", event: unsupportedEngineEvent{}},
		{name: "invalid terminal outcome", event: SegmentEnded{Reason: run.Outcome(255)}},
		{name: "malformed interrupt", event: SegmentInterrupted{Interrupts: []Interrupt{{Kind: interrupt.Kind(9)}}}},
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
		_, err := reducer.reduce(ToolCallStarted{
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
		mustReduce(t, reducer, ToolCallStarted{
			CallID: "call_1", ToolName: "shell", Arguments: `{"command":"go test","description":"Run tests"}`,
		})
		_, err := reducer.reduce(ToolCallFinished{CallID: "call_1", Arguments: "null"})
		if !errors.Is(err, errExecutorContract) || !errors.Is(err, tool.ErrInvalidArguments) {
			t.Fatalf("tool end error = %v, want executor protocol + invalid arguments", err)
		}
	})
}

func TestReducerRejectsInvalidToolLifecycle(t *testing.T) {
	tests := []struct {
		name  string
		event ExecutionFact
		want  string
	}{
		{name: "missing call id", event: ToolCallStarted{ToolName: "shell"}, want: "id is required"},
		{name: "missing tool name", event: ToolCallStarted{CallID: "call_1"}, want: "name is required"},
		{name: "end without start", event: ToolCallFinished{CallID: "call_1"}, want: "without an open start"},
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
		start := ToolCallStarted{CallID: "call_1", ToolName: "shell", Arguments: `{}`}
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
	config.Continuation = testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []Continuation{{
			RunID: "run_1",
			DrainedTools: []DrainedTool{{
				ItemID: "item_original", ItemOccurredAt: itemOccurredAt, CallID: "old_call", Name: "lookup",
				Arguments: `{"b":2,"a":{"enabled":true}}`,
			}},
		}},
	})
	reducer := newReducer(config)
	started := mustReduce(t, reducer, ToolCallStarted{
		CallID: "new_call", ToolName: "lookup",
		Arguments: "{\n  \"a\": {\"enabled\": true}, \"b\": 2\n}",
	})
	for _, reduction := range started {
		if _, ok := reduction.Event.(ItemStarted); ok {
			t.Fatal("resumed canonical Tool Item published a second lifecycle start")
		}
	}
	if itemID, open := reducer.openToolItemID("new_call"); !open || itemID != "item_original" {
		t.Fatalf("resumed open tool = %q/%t, want item_original", itemID, open)
	}
	completed := completedItem(t, mustReduce(t, reducer, ToolCallFinished{CallID: "new_call", Result: testToolResult(t, "ok")}))
	if completed.ID() != "item_original" || !completed.OccurredAt().Equal(itemOccurredAt) {
		t.Fatalf("resumed completed tool = %s/%s, want original identity and occurrence", completed.ID(), completed.OccurredAt())
	}
}

func TestReducerTerminalSynthesisClosesUnrestartedResumeTool(t *testing.T) {
	itemOccurredAt := time.Unix(1, 0).UTC()
	config := testReducerConfig()
	config.Continuation = testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []Continuation{{
			RunID: "run_1",
			DrainedTools: []DrainedTool{{
				ItemID: "item_original", ItemOccurredAt: itemOccurredAt,
				CallID: "old_call", SourceCallID: "provider_original",
				Name: "shell", Arguments: `{"command":"pwd"}`,
			}},
		}},
	})
	reducer := newReducer(config)
	if _, err := reducer.open(); err != nil {
		t.Fatalf("open resumed segment: %v", err)
	}

	batch, err := reducer.synthesizeTerminal()
	if err != nil {
		t.Fatalf("synthesize terminal: %v", err)
	}
	var settled transcript.Item
	for _, reduced := range batch.events {
		if completed, ok := reduced.Event.(ItemCompleted); ok && completed.Item.ID() == "item_original" {
			settled = completed.Item
		}
	}
	if settled.ID() == "" || settled.Status() != transcript.ItemIncomplete {
		t.Fatalf("resumed tool terminal item = %+v, want incomplete item_original", settled.Snapshot())
	}
	if duration, known := settled.ExecutionDuration(); known {
		t.Fatalf("unrestarted resume Tool fabricated execution duration %v", duration)
	}
	closure := committedConversationMessages(testReductions(batch))
	if len(closure) != 1 || len(closure[0].Parts) != 1 ||
		closure[0].Parts[0].ToolResult == nil ||
		closure[0].Parts[0].ToolResult.ID != "provider_original" ||
		!closure[0].Parts[0].ToolResult.IsError {
		t.Fatalf("unrestarted resume Tool conversation closure = %#v", closure)
	}
}

func TestReducerRejectsMalformedDurableResumeArguments(t *testing.T) {
	config := testReducerConfig()
	config.Continuation = testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []Continuation{{
			RunID: "run_1",
			DrainedTools: []DrainedTool{{
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
	config.Continuation = testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []Continuation{{
			RunID: "run_1",
			CommittedTools: []CommittedTool{{
				ItemID: "item_child", CallID: "call_child", Name: "delegate_task", Arguments: "{}",
				Failure: tool.Failure{
					Kind:   tool.FailureChildRunCanceled,
					Detail: "stop delegated branch",
				},
			}},
		}},
	})
	reducer := newReducer(config)
	if _, err := reducer.open(); err != nil {
		t.Fatalf("open: %v", err)
	}

	batch, err := reducer.reduce(ToolCallFinished{
		CallID:    "call_child",
		Arguments: "{}",
		Failure: &tool.Failure{
			Kind:   tool.FailureExecution,
			Detail: "delegated child canceled",
		},
	})
	if err != nil {
		t.Fatalf("consume committed ToolCallFinished: %v", err)
	}
	if len(batch.events) != 0 || batch.parkCommit != nil {
		t.Fatalf("committed tool result projected duplicate events: %+v", batch)
	}
	if remaining := reducer.resume.remainingCommittedTools(); len(remaining) != 0 {
		t.Fatalf("remaining committed tools = %+v, want none", remaining)
	}
}

func TestReducerRejectsReexecutionOrSuccessForHostCommittedTool(t *testing.T) {
	continuation := testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1",
		Continuations: []Continuation{{
			RunID: "run_1",
			CommittedTools: []CommittedTool{{
				ItemID: "item_child", CallID: "call_child", Name: "delegate_task", Arguments: "{}",
				Failure: tool.Failure{
					Kind: tool.FailureChildRunCanceled,
				},
			}},
		}},
	})
	t.Run("reexecution", func(t *testing.T) {
		config := testReducerConfig()
		config.Continuation = continuation
		reducer := newReducer(config)
		_, err := reducer.reduce(ToolCallStarted{
			CallID: "call_child", ToolName: "delegate_task", Arguments: "{}",
		})
		if !errors.Is(err, errExecutorContract) ||
			!strings.Contains(err.Error(), "executed again") {
			t.Fatalf("ToolCallStarted error = %v, want committed-call reexecution violation", err)
		}
	})
	t.Run("successful result", func(t *testing.T) {
		config := testReducerConfig()
		config.Continuation = continuation
		reducer := newReducer(config)
		_, err := reducer.reduce(ToolCallFinished{CallID: "call_child", Arguments: "{}"})
		if !errors.Is(err, errExecutorContract) ||
			!strings.Contains(err.Error(), "successful result") {
			t.Fatalf("ToolCallFinished error = %v, want committed-call success violation", err)
		}
	})
}

func TestReducerRejectsInvalidInterruptProjection(t *testing.T) {
	waiting := SegmentFinished{Run: runfixture.MustRestore(run.Snapshot{State: run.Waiting})}
	tests := []struct {
		name   string
		events []RunEvent
	}{
		{
			name:   "multiple interrupt boundaries",
			events: []RunEvent{waiting, waiting},
		},
		{
			name: "additional lifecycle transition",
			events: []RunEvent{
				SegmentStarted{Run: runfixture.MustRestore(run.Snapshot{State: run.Running})},
				waiting,
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
		run := runfixture.MustRestore(run.Snapshot{State: run.Waiting})
		return &EventCommit{SegmentID: "segment_1",
			State: StateSuspend,
			Run:   &run,
		}
	}
	terminalCommit := func() *EventCommit {
		outcome := run.OutcomeCompleted
		run := runfixture.MustRestore(run.Snapshot{State: run.Completed, Outcome: &outcome})
		return &EventCommit{SegmentID: "segment_1", State: StateTerminalize, Outcome: outcome, Run: &run}
	}
	invalidTerminalCommit := terminalCommit()
	invalidTerminalCommit.Outcome = run.OutcomeFailed
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
			Event: ItemCompleted{}, Commit: &EventCommit{SegmentID: "segment_1", State: StateChange(255)},
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

func TestCombineTerminalEventCommitRejectsSegmentOwnershipChange(t *testing.T) {
	var terminalRun run.Run
	_, err := combineTerminalEventCommit(reductionBatch{events: []reduction{
		{Commit: &EventCommit{RunID: "run_1", SessionID: "ses_1", SegmentID: "segment_old"}},
		{Commit: &EventCommit{
			RunID: "run_1", SessionID: "ses_1", SegmentID: "segment_new",
			State: StateTerminalize, Run: &terminalRun,
		}},
	}})
	if err == nil {
		t.Fatal("combined terminal batch changed Segment ownership")
	}
}

func TestValidateRouteCommitRejectsSegmentOwnershipChange(t *testing.T) {
	err := validateRouteCommit(
		&executorRoute{runID: "run_1", segmentID: "segment_new"},
		"ses_1",
		&EventCommit{RunID: "run_1", SessionID: "ses_1", SegmentID: "segment_old"},
	)
	if !errors.Is(err, errReducerInvariant) {
		t.Fatalf("validateRouteCommit error = %v, want reducer invariant", err)
	}
}

func TestReducerDrainsToolsInStartOrder(t *testing.T) {
	reducer := newReducer(testReducerConfig())
	for _, event := range []ToolCallStarted{
		{CallID: "call_z", ToolName: "first", Arguments: `{}`},
		{CallID: "call_a", ToolName: "second", Arguments: `{}`},
		{CallID: "call_m", ToolName: "third", Arguments: `{}`},
	} {
		mustReduce(t, reducer, event)
	}

	drained := drainedToolRefs(reducer.tools.ordered(), nil, nil)
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
		invocation, _ := event.(ItemCompleted).Item.ToolInvocation()
		got = append(got, invocation.Name)
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

func completedItems(reductions []reduction) []transcript.Item {
	var items []transcript.Item
	for _, reduction := range reductions {
		if event, ok := reduction.Event.(ItemCompleted); ok {
			items = append(items, event.Item)
		}
	}
	return items
}

func startedItemID(t *testing.T, reductions []reduction) string {
	t.Helper()
	for _, reduction := range reductions {
		if event, ok := reduction.Event.(ItemStarted); ok {
			return event.Item.ItemID
		}
	}
	t.Fatalf("no ItemStarted in %+v", reductions)
	return ""
}

func completedToolNames(reductions []reduction) []string {
	var names []string
	for _, reduction := range reductions {
		if event, ok := reduction.Event.(ItemCompleted); ok {
			if invocation, present := event.Item.ToolInvocation(); present {
				names = append(names, invocation.Name)
			}
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
	frozen := run.Capabilities{
		ChildRuns:      true,
		InterruptKinds: []interrupt.Kind{interrupt.Approval},
	}
	config := testReducerConfig()
	config.Capabilities = frozen
	config.Continuation = testTreeContinuation(Pending{
		RootRunID: "run_1", SessionID: "ses_1", Capabilities: frozen,
	})

	reducer := newReducer(config)
	opening := mustOpen(t, reducer)
	started, ok := opening[0].Event.(SegmentStarted)
	if !ok {
		t.Fatalf("opening event = %#v, want SegmentStarted", opening[0].Event)
	}
	assertFrozenCapabilities(t, started.Run.Capabilities(), frozen, "segment.started")

	batch := mustReduceBatch(t, reducer, SegmentInterrupted{Interrupts: []Interrupt{
		{Kind: interrupt.Approval, Approval: &ApprovalPrompt{
			ToolName: "shell", Arguments: `{}`, SafetyClass: "exec", Risk: "high",
		}},
	}})
	if batch.parkCommit.Run == nil {
		t.Fatal("park commit carries no run record")
	}
	assertFrozenCapabilities(t, batch.parkCommit.Run.Capabilities(), frozen, "parked run record")
}

func assertFrozenCapabilities(t *testing.T, got, want run.Capabilities, where string) {
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
	mustReduce(t, reducer, PlanUpdated{State: testPlanState(t, plan.Snapshot{
		Steps:    []plan.Step{{Description: "fence this", Status: plan.StatusInProgress}},
		Revision: 4, UpdatedAt: time.Unix(11, 0).UTC(),
	})})

	terminal := mustReduce(t, reducer, SegmentEnded{Reason: run.OutcomeCompleted})
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
	quiet := mustReduce(t, untouched, SegmentEnded{Reason: run.OutcomeCompleted})
	for _, reduced := range quiet {
		if snapshot, ok := reduced.Event.(StateSnapshot); ok {
			t.Fatalf("a segment that changed no state published %+v", snapshot)
		}
	}
}
