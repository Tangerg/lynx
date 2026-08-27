package runs

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/accounting"
	"github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/tool"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

type authoritativeFailureExecutor struct {
	mu       sync.Mutex
	released int
	receipts chan error
}

type concurrentToolExecutor struct {
	failures chan error
}

func (c *concurrentToolExecutor) Observe(
	ctx context.Context,
	_ ExecutorRef,
) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		member := ExecutorMember{MemberID: "member_root"}
		starts := []ToolCallStarted{
			{CallID: "tool_first", SourceCallID: "provider_first", ModelCallSequence: 1, ToolCallIndex: 0, ToolName: "first", Arguments: `{}`},
			{CallID: "tool_second", SourceCallID: "provider_second", ModelCallSequence: 1, ToolCallIndex: 1, ToolName: "second", Arguments: `{}`},
		}
		for _, fact := range starts {
			commit, receipt, err := NewExecutionFactCommit(fact)
			if err != nil || !yield(ExecutorEvent{Member: member, Payload: commit}) {
				return
			}
			if err := receipt.Await(ctx); err != nil {
				c.failures <- err
				return
			}
		}

		second, secondReceipt, err := NewExecutionFactCommit(ToolCallFinished{
			CallID: "tool_second", Result: toolStringResult("second-result"),
		})
		if err != nil || !yield(ExecutorEvent{Member: member, Payload: second}) {
			return
		}
		first, firstReceipt, err := NewExecutionFactCommit(ToolCallFinished{
			CallID: "tool_first", Result: toolStringResult("first-result"),
		})
		if err != nil || !yield(ExecutorEvent{Member: member, Payload: first}) {
			return
		}
		firstErr := firstReceipt.Await(ctx)
		secondErr := secondReceipt.Await(ctx)
		c.failures <- errors.Join(firstErr, secondErr)
		if firstErr != nil || secondErr != nil {
			yield(ExecutorEvent{
				Member:  member,
				Payload: UnknownEffectsDetected{IDs: []string{"effect_tools"}},
			})
			return
		}
		yield(ExecutorEvent{Member: member, Payload: SegmentEnded{Reason: run.OutcomeCompleted}})
	}, nil
}

func (c *concurrentToolExecutor) Release(context.Context, ExecutorRef) error { return nil }

func toolStringResult(value string) *tool.Result {
	result := tool.StringResult(value)
	return &result
}

func (a *authoritativeFailureExecutor) Observe(
	ctx context.Context,
	_ ExecutorRef,
) (iter.Seq[ExecutorEvent], error) {
	return func(yield func(ExecutorEvent) bool) {
		member := ExecutorMember{MemberID: "member_root"}
		start, startReceipt, err := NewExecutionFactCommit(ModelCallStarted{CallID: "model_call_1"})
		if err != nil || !yield(ExecutorEvent{Member: member, Payload: start}) {
			return
		}
		a.receipts <- startReceipt.Await(ctx)

		usage := accounting.TokenUsage{PromptTokens: 2, CompletionTokens: 1}
		completion, completionReceipt, err := NewExecutionFactCommit(ModelCallCompleted{
			CallID: "model_call_1",
			Message: corechat.NewAssistantMessage(
				corechat.NewTextPart("durable final that must not be half-published"),
			),
			TokenUsage: usage,
			ByModel: []accounting.ModelUsage{{
				Model: "test-model", TokenUsage: usage, Calls: 1,
			}},
			Steps: 1,
		})
		if err != nil || !yield(ExecutorEvent{Member: member, Payload: completion}) {
			return
		}
		a.receipts <- completionReceipt.Await(ctx)
		yield(ExecutorEvent{
			Member: member,
			Payload: UnknownEffectsDetected{
				IDs: []string{"effect_1"},
			},
		})
	}, nil
}

func (a *authoritativeFailureExecutor) Release(context.Context, ExecutorRef) error {
	a.mu.Lock()
	a.released++
	a.mu.Unlock()
	return nil
}

func (a *authoritativeFailureExecutor) releaseCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.released
}

func TestAuthoritativeProjectionFailurePreservesStartUntilAtomicRunLost(t *testing.T) {
	executor := &authoritativeFailureExecutor{receipts: make(chan error, 2)}
	writeFailure := errors.New("authoritative transaction unavailable")
	effects := &fakeEffects{commitErr: writeFailure, commitErrAt: 2}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	if startErr := <-executor.receipts; startErr != nil {
		t.Fatalf("model start receipt = %v, want success", startErr)
	}
	if completionErr := <-executor.receipts; !errors.Is(completionErr, writeFailure) {
		t.Fatalf("model completion receipt = %v, want %v", completionErr, writeFailure)
	}

	commits := effects.commitSnapshot()
	if len(commits) != 2 {
		t.Fatalf("committed write-sets = %d, want start + atomic lost", len(commits))
	}
	if commits[0].CommitID == "" || commits[1].CommitID == "" || commits[0].CommitID == commits[1].CommitID {
		t.Fatalf("commit identities = %q, %q; want distinct non-empty write-set identities", commits[0].CommitID, commits[1].CommitID)
	}
	if got := commits[0].ModelInvocations; len(got) != 1 || got[0].State != ModelInvocationStarted {
		t.Fatalf("first commit model invocations = %#v", got)
	}
	lost := commits[1]
	if lost.State != StateTerminalize || lost.Outcome != run.OutcomeLost || lost.Run == nil ||
		!runHasOutcome(*lost.Run, run.OutcomeLost) {
		t.Fatalf("lost commit = %#v", lost)
	}
	if got := lost.ModelInvocations; len(got) != 1 || got[0].State != ModelInvocationUnknown {
		t.Fatalf("lost model invocations = %#v", got)
	}
	for _, commit := range commits {
		for _, item := range commit.Items {
			if item.Kind() == transcript.AgentMessage && len(item.Content()) > 0 {
				t.Fatalf("failed final projection leaked a completed assistant item: %#v", item)
			}
		}
	}
	if len(events) == 0 {
		t.Fatal("Run stream is empty")
	}
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || !runHasOutcome(finished.Run, run.OutcomeLost) {
		t.Fatalf("last event = %#v, want lost SegmentFinished", events[len(events)-1].Payload)
	}
	if executor.releaseCount() != 1 {
		t.Fatalf("executor releases = %d, want 1 after durable lost", executor.releaseCount())
	}
}

func TestUnknownRunLostRetriesDurableTerminalBeforeExecutorRelease(t *testing.T) {
	executor := &authoritativeFailureExecutor{receipts: make(chan error, 2)}
	terminalAttempts := make(chan struct{}, 2)
	writeFailure := errors.New("authoritative transaction unavailable")
	effects := &fakeEffects{
		commitErr: writeFailure, commitErrAt: 2, commitErrCount: 2,
		terminalStarted: terminalAttempts,
	}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	streamDone := make(chan []Event, 1)
	go func() { streamDone <- collectEvents(stream) }()
	select {
	case <-terminalAttempts:
	case <-time.After(time.Second):
		t.Fatal("RunLost terminal attempt did not start")
	}
	if executor.releaseCount() != 0 {
		t.Fatalf("executor released before durable RunLost retry: %d", executor.releaseCount())
	}
	select {
	case <-terminalAttempts:
	case <-time.After(time.Second):
		t.Fatal("RunLost terminal was not retried")
	}
	events := <-streamDone
	if executor.releaseCount() != 1 {
		t.Fatalf("executor releases after durable RunLost = %d, want 1", executor.releaseCount())
	}
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || !runHasOutcome(finished.Run, run.OutcomeLost) {
		t.Fatalf("last event = %#v, want durable RunLost", events[len(events)-1].Payload)
	}
}

func runHasOutcome(record run.Run, expected run.Outcome) bool {
	outcome, terminal := record.Outcome()
	return terminal && outcome == expected
}

func TestConcurrentToolResultsCommitInModelOrder(t *testing.T) {
	executor := &concurrentToolExecutor{failures: make(chan error, 1)}
	effects := &fakeEffects{}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	collectEvents(stream)
	if err := <-executor.failures; err != nil {
		t.Fatalf("Tool result receipts: %v", err)
	}

	commits := effects.commitSnapshot()
	seenCommitIDs := make(map[string]struct{}, len(commits))
	for index, commit := range commits {
		if commit.CommitID == "" {
			t.Fatalf("commit[%d] has no write-set identity", index)
		}
		if _, duplicate := seenCommitIDs[commit.CommitID]; duplicate {
			t.Fatalf("commit[%d] repeats write-set identity %q", index, commit.CommitID)
		}
		seenCommitIDs[commit.CommitID] = struct{}{}
	}
	for index, name := range []string{"first", "second"} {
		commit := commits[index]
		if len(commit.Items) != 1 || commit.Items[0].Status() != transcript.ItemRunning {
			t.Fatalf("Tool start[%d] Items = %#v, want one running Item", index, commit.Items)
		}
		invocation, present := commit.Items[0].ToolInvocation()
		if !present || invocation.Name != name || len(commit.ToolInvocations) != 1 ||
			commit.ToolInvocations[0].State != ToolInvocationStarted ||
			commit.ToolInvocations[0].ItemID != commit.Items[0].ID() {
			t.Fatalf("Tool start[%d] = Item:%#v journal:%#v", index, commit.Items[0], commit.ToolInvocations)
		}
	}

	var final *EventCommit
	for _, commit := range commits {
		if len(commit.ToolInvocations) == 2 && len(commit.Items) == 2 {
			cloned := commit
			final = &cloned
			break
		}
	}
	if final == nil {
		t.Fatalf("no canonical Tool batch in commits: %#v", commits)
	}
	first, firstPresent := final.Items[0].ToolInvocation()
	second, secondPresent := final.Items[1].ToolInvocation()
	if !firstPresent || first.Name != "first" || !secondPresent || second.Name != "second" {
		t.Fatalf("Tool Item order = %#v, want first then second", final.Items)
	}
	if final.ToolInvocations[0].CallID != "tool_first" ||
		final.ToolInvocations[1].CallID != "tool_second" {
		t.Fatalf("Tool invocation order = %#v", final.ToolInvocations)
	}
	if len(final.ConversationMessages) != 1 || final.ConversationMessages[0].Role != corechat.RoleTool ||
		len(final.ConversationMessages[0].Parts) != 2 ||
		final.ConversationMessages[0].Parts[0].ToolResult == nil ||
		final.ConversationMessages[0].Parts[0].ToolResult.ID != "provider_first" ||
		final.ConversationMessages[0].Parts[1].ToolResult == nil ||
		final.ConversationMessages[0].Parts[1].ToolResult.ID != "provider_second" {
		t.Fatalf("Tool conversation projection = %#v, want provider-ordered results", final.ConversationMessages)
	}
}

func TestConcurrentToolBatchFailurePublishesOnlyIncompleteRunLost(t *testing.T) {
	executor := &concurrentToolExecutor{failures: make(chan error, 1)}
	writeFailure := errors.New("canonical Tool batch unavailable")
	effects := &fakeEffects{commitErr: writeFailure, commitErrAt: 3}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	collectEvents(stream)
	if receiptErr := <-executor.failures; !errors.Is(receiptErr, writeFailure) {
		t.Fatalf("Tool result receipt error = %v, want %v", receiptErr, writeFailure)
	}

	commits := effects.commitSnapshot()
	if len(commits) != 3 {
		t.Fatalf("committed write-sets = %d, want two starts + RunLost", len(commits))
	}
	lost := commits[2]
	if lost.State != StateTerminalize || lost.Outcome != run.OutcomeLost {
		t.Fatalf("terminal commit = %#v, want RunLost", lost)
	}
	if len(lost.Items) != 2 || len(lost.ToolInvocations) != 2 {
		t.Fatalf("lost Tool projection = items %#v invocations %#v", lost.Items, lost.ToolInvocations)
	}
	for index := range lost.Items {
		invocation, present := lost.Items[index].ToolInvocation()
		if lost.Items[index].Status() != transcript.ItemIncomplete || !present ||
			invocation.Result != nil || lost.ToolInvocations[index].State != ToolInvocationIncomplete {
			t.Fatalf("lost Tool[%d] leaked a result: item %#v invocation %#v", index, lost.Items[index], lost.ToolInvocations[index])
		}
	}
}

func TestTerminalTransactionFailurePreservesRunningToolsForAtomicRecovery(t *testing.T) {
	member := ExecutorMember{MemberID: "member_root"}
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{Member: member, Payload: ToolCallStarted{
			CallID: "tool_first", SourceCallID: "provider_first",
			ModelCallSequence: 1, ToolCallIndex: 0, ToolName: "first", Arguments: `{}`,
		}},
		{Member: member, Payload: ToolCallStarted{
			CallID: "tool_second", SourceCallID: "provider_second",
			ModelCallSequence: 1, ToolCallIndex: 1, ToolName: "second", Arguments: `{}`,
		}},
		{Member: member, Payload: SegmentEnded{Reason: run.OutcomeCompleted}},
	}}
	writeFailure := errors.New("terminal transaction unavailable")
	effects := &fakeEffects{commitErr: writeFailure, commitErrAt: 3}
	coordinator := testCoordinator(executor, effects)

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)

	commits := effects.commitSnapshot()
	if len(commits) != 3 {
		t.Fatalf("committed write-sets = %d, want two starts + recovered terminal", len(commits))
	}
	terminal := commits[2]
	if terminal.State != StateTerminalize || terminal.Outcome != run.OutcomeFailed || terminal.Run == nil ||
		!runHasFailureKind(*terminal.Run, run.FailureInternal) {
		t.Fatalf("recovered terminal = %#v, want internal failure", terminal)
	}
	if len(terminal.Items) != 2 || len(terminal.ToolInvocations) != 2 {
		t.Fatalf("recovered Tool write-set = items %#v invocations %#v", terminal.Items, terminal.ToolInvocations)
	}
	for index := range terminal.Items {
		if terminal.Items[index].Status() != transcript.ItemIncomplete ||
			terminal.ToolInvocations[index].State != ToolInvocationIncomplete {
			t.Fatalf("recovered Tool[%d] = item %#v journal %#v", index, terminal.Items[index], terminal.ToolInvocations[index])
		}
	}
	terminalEvents := 0
	for _, event := range events {
		if finished, ok := event.Payload.(SegmentFinished); ok {
			terminalEvents++
			if !runHasFailureKind(finished.Run, run.FailureInternal) {
				t.Fatalf("published terminal = %#v, want recovered internal failure", finished.Run)
			}
		}
	}
	if terminalEvents != 1 {
		t.Fatalf("published terminal events = %d, want exactly one recovered boundary", terminalEvents)
	}
}
