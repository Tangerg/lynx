package agentexec

import (
	"context"
	"errors"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/lynx/app/runtime/internal/adapter/toolset"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	domaintool "github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	toolcontract "github.com/Tangerg/lynx/tool"
)

func TestInteractionExecutorProjectsAuthoritativeModelToolLifecycleAndAccounting(t *testing.T) {
	type echoInput struct {
		Value string `json:"value"`
	}
	var toolCalls int
	echo, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "echo", Description: "Return the supplied value.",
	}, func(_ context.Context, input echoInput) (string, error) {
		toolCalls++
		return input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "provider_call", Name: "echo", Arguments: `{"value":"hello"}`}, 7, 2),
		interactionUsageTextResponse("done", 11, 3),
	}}
	hooks := &recordingInteractionHooks{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{echo}}},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolPresenter: testInteractionToolPresenter{},
		ToolAuthorizer: allowInteractionTools{}, ToolHooks: hooks,
		Pricing: func(_, _ string, _ *chat.Usage) float64 { return 0.25 }, Provider: "test",
	})

	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)
	if toolCalls != 1 {
		t.Fatalf("Tool calls = %d, want 1", toolCalls)
	}
	if got := len(payloadsOf[runs.ModelCallStarted](events)); got != 2 {
		t.Fatalf("model starts = %d, want 2", got)
	}
	models := payloadsOf[runs.ModelCallCompleted](events)
	if len(models) != 2 || models[1].Steps != 2 || models[1].TokenUsage.PromptTokens != 18 ||
		models[1].TokenUsage.CompletionTokens != 5 || models[1].CostUSD != 0.5 {
		t.Fatalf("model completions = %#v", models)
	}
	starts := payloadsOf[runs.ToolCallStarted](events)
	finishes := payloadsOf[runs.ToolCallFinished](events)
	if len(starts) != 1 || len(finishes) != 1 || starts[0].SourceCallID != "provider_call" ||
		starts[0].Activity != "Echoing value" || starts[0].SafetyClass != domaintool.SafetyClassSafe ||
		finishes[0].Result == nil || finishes[0].Failure != nil {
		t.Fatalf("Tool lifecycle = starts %#v; finishes %#v", starts, finishes)
	}
	if hooks.before != 1 || hooks.after != 1 {
		t.Fatalf("hook calls = before %d after %d", hooks.before, hooks.after)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Usage == nil || ended[0].Usage.Steps != 2 ||
		ended[0].Usage.Tokens.PromptTokens != 18 || ended[0].Usage.CostUSD != 0.5 {
		t.Fatalf("segment accounting = %#v", ended)
	}
}

func TestInteractionExecutorCancellationStopsCooperativeInflightTool(t *testing.T) {
	toolStarted := make(chan struct{})
	toolReturned := make(chan struct{})
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "block", Description: "Block until canceled.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		close(toolStarted)
		<-ctx.Done()
		close(toolReturned)
		return "", ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "provider_call", Name: "block", Arguments: `{}`}, 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executor.Release(context.Background(), ref); err != nil {
			t.Errorf("Release: %v", err)
		}
	})
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		for event := range sequence {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(nil)
				event.Payload = commit.Fact
			}
			events = append(events, event)
		}
		eventsReady <- events
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	select {
	case <-toolStarted:
	case <-time.After(time.Second):
		t.Fatal("Tool did not start")
	}
	if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-toolReturned:
	case <-time.After(time.Second):
		t.Fatal("Tool context was not canceled")
	}
	var events []runs.ExecutorEvent
	select {
	case events = <-eventsReady:
	case <-time.After(time.Second):
		t.Fatal("canceled Interaction did not reach a terminal boundary")
	}
	if unknown := payloadsOf[runs.UnknownEffectsDetected](events); len(unknown) != 0 {
		t.Fatalf("canceled Tool became an unknown Effect: %#v", unknown)
	}
	finished := payloadsOf[runs.ToolCallFinished](events)
	if len(finished) != 1 || finished[0].Failure == nil ||
		finished[0].Failure.Kind != domaintool.FailureCanceled ||
		finished[0].Failure.Detail != "" {
		t.Fatalf("canceled Tool completion = %#v", finished)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("segment end = %#v, want canceled", ended)
	}
}

func TestInteractionExecutorCancellationStopsCooperativeInflightModel(t *testing.T) {
	modelStarted := make(chan struct{})
	modelReturned := make(chan struct{})
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		close(modelStarted)
		<-ctx.Done()
		close(modelReturned)
		return nil, ctx.Err()
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executor.Release(context.Background(), ref); err != nil {
			t.Errorf("Release: %v", err)
		}
	})
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		for event := range sequence {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(nil)
				event.Payload = commit.Fact
			}
			events = append(events, event)
		}
		eventsReady <- events
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	select {
	case <-modelStarted:
	case <-time.After(time.Second):
		t.Fatal("model did not start")
	}
	if err := executor.RequestRootCancellation(t.Context(), ref, "operator canceled"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-modelReturned:
	case <-time.After(time.Second):
		t.Fatal("model context was not canceled")
	}
	var events []runs.ExecutorEvent
	select {
	case events = <-eventsReady:
	case <-time.After(time.Second):
		t.Fatal("canceled Interaction did not reach a terminal boundary")
	}
	if unknown := payloadsOf[runs.UnknownEffectsDetected](events); len(unknown) != 0 {
		t.Fatalf("canceled model became an unknown Effect: %#v", unknown)
	}
	if failed := payloadsOf[runs.ModelCallFailed](events); len(failed) != 1 {
		t.Fatalf("model failures = %#v, want one definite failed invocation", failed)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("segment end = %#v, want canceled", ended)
	}
}

func TestInteractionExecutorBindsResolvedRunScopeToManifestAndToolCalls(t *testing.T) {
	start := interactionTestStart()
	start.CWD = "/isolated/project"
	start.WorkspaceCWD = "/workspace/project"
	start.Isolated = true
	start.GoalIncarnationID = "goal_lease"
	want := rootExecutionScope(start)
	var toolScope runs.ExecutionScope
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "scope", Description: "Return the current execution scope.",
	}, func(ctx context.Context, _ struct{}) (string, error) {
		toolScope, _ = executionctx.Scope(ctx)
		return "ok", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &scopeRecordingInteractionTools{manifest: toolset.Manifest{
		Visible: []toolcontract.Tool{executable},
	}}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "scope_call", Name: "scope", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("done", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: resolver, ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer: allowInteractionTools{},
	})
	runInteractionHarness(context.Background(), t, executor, start, nil)
	if !resolver.ok || resolver.scope != want {
		t.Fatalf("manifest scope = (%+v, %t), want %+v", resolver.scope, resolver.ok, want)
	}
	if toolScope != want {
		t.Fatalf("Tool scope = %+v, want %+v", toolScope, want)
	}
}

func TestInteractionExecutorChunkDropPreservesFinalAndUsage(t *testing.T) {
	const chunks = 256
	model := streamingObservationModel{chunks: chunks, streamed: make(chan struct{})}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		StreamModelResponses: true,
		DeltaBufferCapacity:  1,
	})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		blocked := false
		for event := range sequence {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(nil)
				event.Payload = commit.Fact
			}
			events = append(events, event)
			if _, delta := event.Payload.(runs.MessageDelta); delta && !blocked {
				blocked = true
				<-model.streamed
			}
		}
		eventsReady <- events
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	events := <-eventsReady
	if err := executor.Release(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	deltas := payloadsOf[runs.MessageDelta](events)
	if len(deltas) >= chunks {
		t.Fatalf("streaming deltas = %d, want an observable bounded-buffer drop below %d", len(deltas), chunks)
	}
	completionSeen := false
	for _, event := range events {
		switch event.Payload.(type) {
		case runs.ModelCallCompleted:
			completionSeen = true
		case runs.MessageDelta:
			if completionSeen {
				t.Fatal("accepted stream Delta arrived after authoritative model completion")
			}
		}
	}
	completed := payloadsOf[runs.ModelCallCompleted](events)
	if len(completed) != 1 || completed[0].Message.Text() != strings.Repeat("x", chunks) ||
		completed[0].TokenUsage.PromptTokens != 5 || completed[0].TokenUsage.CompletionTokens != 2 {
		t.Fatalf("authoritative model completion = %#v", completed)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Usage == nil || ended[0].Usage.Tokens.PromptTokens != 5 ||
		ended[0].Usage.Tokens.CompletionTokens != 2 {
		t.Fatalf("terminal usage = %#v", ended)
	}
}

func TestInteractionExecutorCommitsDeferredAdvertisementThroughAgentFramework(t *testing.T) {
	hidden, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "hidden_lookup", Description: "Read a hidden value.",
	}, func(context.Context, struct{}) (string, error) { return "found", nil })
	if err != nil {
		t.Fatal(err)
	}
	search, err := toolset.NewDiscovery([]toolcontract.Tool{hidden})
	if err != nil {
		t.Fatal(err)
	}
	model := &manifestScriptModel{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{search}, Deferred: []toolcontract.Tool{hidden},
		}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)
	if want := [][]string{{"search_tools"}, {"search_tools", "hidden_lookup"}, {"search_tools", "hidden_lookup"}}; !slices.EqualFunc(model.manifests, want, slices.Equal[[]string]) {
		t.Fatalf("model manifests = %v, want %v", model.manifests, want)
	}
	starts := payloadsOf[runs.ToolCallStarted](events)
	if len(starts) != 2 || starts[0].ToolName != "search_tools" || starts[1].ToolName != "hidden_lookup" {
		t.Fatalf("Tool starts = %#v", starts)
	}
}

func TestInteractionExecutorKeepsRefetchableProjectionAndPostHookObservational(t *testing.T) {
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "observe", Description: "Return a value.",
	}, func(context.Context, struct{}) (string, error) { return "value", nil })
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "observe_call", Name: "observe", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("done", 1, 1),
	}}
	hooks := &failingAfterInteractionHooks{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{
			Visible: []toolcontract.Tool{executable},
		}},
		ToolInterpreter: failingOutcomeInteractionInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
		ToolHooks:       hooks,
	})
	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)
	if hooks.after != 1 {
		t.Fatalf("post-Tool hooks = %d, want 1", hooks.after)
	}
	if len(payloadsOf[runs.UnknownEffectsDetected](events)) != 0 {
		t.Fatalf("refetchable projection or observational hook made Effect unknown: %#v", events)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("terminal = %#v, want completed", ended)
	}
}

func TestInteractionExecutorDoesNotCallProviderWhenModelStartCommitFails(t *testing.T) {
	var calls int
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionTextResponse("unexpected"), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if _, start := fact.(runs.ModelCallStarted); start {
			return errors.New("model start store unavailable")
		}
		return nil
	})
	if calls != 0 {
		t.Fatalf("provider calls = %d, want 0", calls)
	}
	if len(payloadsOf[runs.UnknownEffectsDetected](events)) != 0 {
		t.Fatalf("pre-call failure became unknown: %#v", events)
	}
}

func TestInteractionExecutorDoesNotCallToolWhenToolStartCommitFails(t *testing.T) {
	var toolCalls int
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "echo", Description: "Return a value.",
	}, func(context.Context, struct{}) (string, error) {
		toolCalls++
		return "unexpected", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "provider_call", Name: "echo", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("recovered", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if _, start := fact.(runs.ToolCallStarted); start {
			return errors.New("tool start store unavailable")
		}
		return nil
	})
	if toolCalls != 0 {
		t.Fatalf("Tool calls = %d, want 0", toolCalls)
	}
	if len(payloadsOf[runs.UnknownEffectsDetected](events)) != 0 {
		t.Fatalf("pre-Tool failure became unknown: %#v", events)
	}
}

func TestInteractionExecutorReconcilesModelFinalCommitFailureAsUnknown(t *testing.T) {
	var calls int
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionUsageTextResponse("answer", 2, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{})
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if _, complete := fact.(runs.ModelCallCompleted); complete {
			return errors.New("model final store unavailable")
		}
		return nil
	})
	if calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
	unknown := payloadsOf[runs.UnknownEffectsDetected](events)
	if len(unknown) != 1 || len(unknown[0].IDs) != 1 {
		t.Fatalf("unknown observations = %#v, want one Effect", unknown)
	}
	if len(payloadsOf[runs.SegmentEnded](events)) != 0 {
		t.Fatalf("unknown Effect was projected as a definite terminal: %#v", events)
	}
}

func TestInteractionExecutorPollingFindsUnknownWhenDirectWakeIsLost(t *testing.T) {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return interactionUsageTextResponse("answer", 2, 1), nil
	})
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		UnknownEffectPollInterval: 5 * time.Millisecond,
	})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	// A nil wake channel makes the direct notification intentionally lossy while
	// leaving the periodic public-state reconciliation active.
	session.unknownWake = nil
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		for event := range sequence {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				var commitErr error
				if _, completion := commit.Fact.(runs.ModelCallCompleted); completion {
					commitErr = errors.New("model final store unavailable")
				}
				commit.Complete(commitErr)
				event.Payload = commit.Fact
			}
			events = append(events, event)
			if _, unknown := event.Payload.(runs.UnknownEffectsDetected); unknown {
				break
			}
		}
		eventsReady <- events
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	var events []runs.ExecutorEvent
	select {
	case events = <-eventsReady:
	case <-time.After(time.Second):
		t.Fatal("periodic reconciliation did not report unknown Effect")
	}
	if unknown := payloadsOf[runs.UnknownEffectsDetected](events); len(unknown) != 1 || len(unknown[0].IDs) != 1 {
		t.Fatalf("unknown observations = %#v, want polling fallback", unknown)
	}
	if err := executor.Release(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
}

func TestInteractionExecutorReconcilesToolResultCommitFailureAsUnknown(t *testing.T) {
	var toolCalls int
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "echo", Description: "Return a value.",
	}, func(context.Context, struct{}) (string, error) {
		toolCalls++
		return "done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "provider_call", Name: "echo", Arguments: `{}`}, 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if _, complete := fact.(runs.ToolCallFinished); complete {
			return errors.New("Tool result store unavailable")
		}
		return nil
	})
	if toolCalls != 1 {
		t.Fatalf("Tool calls = %d, want 1", toolCalls)
	}
	if unknown := payloadsOf[runs.UnknownEffectsDetected](events); len(unknown) != 1 || len(unknown[0].IDs) != 1 {
		t.Fatalf("unknown observations = %#v, want one Effect", unknown)
	}
	if len(payloadsOf[runs.ToolCallFinished](events)) != 1 {
		t.Fatalf("failed authoritative fact was not observed by the harness: %#v", events)
	}
}

func TestInteractionExecutorPreservesConcurrentToolAttributionWhenCompletionIsOutOfOrder(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}
	allowFirst := make(chan struct{})
	var mu sync.Mutex
	calls := make(map[string]int)
	inner, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "parallel_echo", Description: "Return the supplied value.",
	}, func(_ context.Context, value input) (string, error) {
		mu.Lock()
		calls[value.Value]++
		mu.Unlock()
		if value.Value == "first" {
			<-allowFirst
		}
		return value.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	executable := concurrentInteractionTool{Tool: inner}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolBatchResponse([]chat.ToolCall{
			{ID: "provider_first", Name: "parallel_echo", Arguments: `{"value":"first"}`},
			{ID: "provider_second", Name: "parallel_echo", Arguments: `{"value":"second"}`},
		}, 1, 1),
		interactionUsageTextResponse("done", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:           staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter:        testInteractionToolInterpreter{},
		ToolAuthorizer:         allowInteractionTools{},
		MaxConcurrentToolCalls: 2,
	})
	var release sync.Once
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if finished, ok := fact.(runs.ToolCallFinished); ok && strings.HasSuffix(finished.CallID, ":1") {
			release.Do(func() { close(allowFirst) })
		}
		return nil
	})
	finishes := payloadsOf[runs.ToolCallFinished](events)
	if len(finishes) != 2 || !strings.HasSuffix(finishes[0].CallID, ":1") ||
		!strings.HasSuffix(finishes[1].CallID, ":0") {
		t.Fatalf("Tool completion arrival order = %#v, want second then first", finishes)
	}
	starts := payloadsOf[runs.ToolCallStarted](events)
	byIndex := make(map[uint32]runs.ToolCallStarted, len(starts))
	for _, started := range starts {
		if started.ModelCallSequence != 1 {
			t.Fatalf("Tool attribution = %#v", starts)
		}
		byIndex[started.ToolCallIndex] = started
	}
	if len(starts) != 2 || byIndex[0].SourceCallID != "provider_first" ||
		byIndex[1].SourceCallID != "provider_second" {
		t.Fatalf("Tool attribution = %#v", starts)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["first"] != 1 || calls["second"] != 1 {
		t.Fatalf("Tool calls = %#v", calls)
	}
}

func TestInteractionExecutorMakesWholeConcurrentEffectUnknownWhenOneResultWriteFails(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}
	allowFirst := make(chan struct{})
	allCallsStarted := make(chan struct{})
	var calls int
	var mu sync.Mutex
	inner, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "parallel_write", Description: "Perform one independently safe write.",
	}, func(_ context.Context, value input) (string, error) {
		mu.Lock()
		calls++
		if calls == 2 {
			close(allCallsStarted)
		}
		mu.Unlock()
		// This test exercises a projection failure after every member of the
		// concurrent batch crossed the external boundary. Without this barrier,
		// scheduler order may correctly stop a sibling that never started.
		<-allCallsStarted
		if value.Value == "first" {
			<-allowFirst
		}
		return value.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolBatchResponse([]chat.ToolCall{
			{ID: "write_first", Name: "parallel_write", Arguments: `{"value":"first"}`},
			{ID: "write_second", Name: "parallel_write", Arguments: `{"value":"second"}`},
		}, 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver: staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{
			concurrentInteractionTool{Tool: inner},
		}}},
		ToolInterpreter:        testInteractionToolInterpreter{},
		ToolAuthorizer:         allowInteractionTools{},
		MaxConcurrentToolCalls: 2,
	})
	projectionFailure := errors.New("canonical concurrent Tool batch unavailable")
	var release sync.Once
	events := runInteractionHarnessWithCommit(t, executor, interactionTestStart(), func(fact runs.ExecutionFact) error {
		if finished, ok := fact.(runs.ToolCallFinished); ok && strings.HasSuffix(finished.CallID, ":1") {
			release.Do(func() { close(allowFirst) })
			return projectionFailure
		}
		return nil
	})
	mu.Lock()
	gotCalls := calls
	mu.Unlock()
	if gotCalls != 2 {
		t.Fatalf("external Tool calls = %d, want both exactly once", gotCalls)
	}
	if unknown := payloadsOf[runs.UnknownEffectsDetected](events); len(unknown) != 1 || len(unknown[0].IDs) != 1 {
		t.Fatalf("unknown observations = %#v, want whole Effect unknown", unknown)
	}
	if len(payloadsOf[runs.SegmentEnded](events)) != 0 {
		t.Fatalf("unknown concurrent Effect was projected as definite: %#v", events)
	}
}

func TestInteractionExecutorCommitsAutomaticDenialWithoutCallingTool(t *testing.T) {
	var toolCalls int
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "write", Description: "Write a value.",
	}, func(context.Context, struct{}) (string, error) {
		toolCalls++
		return "unexpected", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "provider_call", Name: "write", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("done", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  denyingInteractionTools{reason: "blocked by automatic policy"},
	})
	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)
	if toolCalls != 0 {
		t.Fatalf("Tool calls = %d, want 0", toolCalls)
	}
	finished := payloadsOf[runs.ToolCallFinished](events)
	if len(finished) != 1 || finished[0].Failure == nil ||
		finished[0].Failure.Kind != domaintool.FailureDenied {
		t.Fatalf("denied Tool completion = %#v", finished)
	}
}

func TestInteractionExecutorPreservesNoProgressDoomLoopBrake(t *testing.T) {
	var toolCalls int
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "lookup", Description: "Return an unchanged value.",
	}, func(context.Context, struct{}) (string, error) {
		toolCalls++
		return "unchanged", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &doomLoopScriptModel{}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:    staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter: testInteractionToolInterpreter{},
		ToolAuthorizer:  allowInteractionTools{},
	})
	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)
	if toolCalls != interactionDoomLoopThreshold {
		t.Fatalf("Tool calls = %d, want %d before brake", toolCalls, interactionDoomLoopThreshold)
	}
	finished := payloadsOf[runs.ToolCallFinished](events)
	if len(finished) != interactionDoomLoopThreshold+1 || finished[len(finished)-1].Failure == nil ||
		finished[len(finished)-1].Failure.Kind != domaintool.FailureDenied {
		t.Fatalf("doom-loop Tool completions = %#v", finished)
	}
}

func TestInteractionExecutorPreservesToolResultOffload(t *testing.T) {
	store := &fakeOffloader{}
	body := strings.Repeat("large result ", 100)
	executable, err := toolcontract.NewFunc(toolcontract.FuncConfig{
		Name: "large", Description: "Return a large value.",
	}, func(context.Context, struct{}) (string, error) { return body, nil })
	if err != nil {
		t.Fatal(err)
	}
	model := &observationScriptModel{responses: []*chat.Response{
		interactionToolResponse(chat.ToolCall{ID: "provider_call", Name: "large", Arguments: `{}`}, 1, 1),
		interactionUsageTextResponse("done", 1, 1),
	}}
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{
		ToolResolver:         staticInteractionTools{manifest: toolset.Manifest{Visible: []toolcontract.Tool{executable}}},
		ToolInterpreter:      testInteractionToolInterpreter{},
		ToolAuthorizer:       allowInteractionTools{},
		ToolResultStore:      store,
		ToolResultThreshold:  100,
		ToolResultReaderName: testToolResultReaderName,
	})
	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)
	finished := payloadsOf[runs.ToolCallFinished](events)
	if store.calls != 1 || len(finished) != 1 || finished[0].Offload == nil ||
		finished[0].Offload.ID != store.lastStage.ID || store.lastStage.SessionID != interactionTestStart().SessionID {
		t.Fatalf("offload calls=%d stage=%#v completions=%#v", store.calls, store.lastStage, finished)
	}
	if finished[0].Result == nil {
		t.Fatal("offloaded Tool result has no inline preview")
	}
	preview, ok := finished[0].Result.String()
	if !ok || !strings.Contains(preview, store.lastStage.ID.String()) || len(preview) >= len(store.lastStage.Body) {
		t.Fatalf("offloaded preview = %q, body bytes = %d", preview, len(store.lastStage.Body))
	}
}

type staticInteractionTools struct{ manifest toolset.Manifest }

var _ InteractionToolResolver = (*toolset.Resolver)(nil)

func (resolver staticInteractionTools) Manifest(context.Context, string) (toolset.Manifest, error) {
	return resolver.manifest, nil
}

type scopeRecordingInteractionTools struct {
	manifest toolset.Manifest
	scope    runs.ExecutionScope
	ok       bool
}

func (resolver *scopeRecordingInteractionTools) Manifest(ctx context.Context, _ string) (toolset.Manifest, error) {
	resolver.scope, resolver.ok = executionctx.Scope(ctx)
	return resolver.manifest, nil
}

type concurrentInteractionTool struct{ toolcontract.Tool }

func (concurrentInteractionTool) ConcurrencyKey(string) (string, bool) { return "", true }

type allowInteractionTools struct{}

func (allowInteractionTools) AuthorizeTool(context.Context, ToolAuthorizationRequest) (ToolAuthorizationDecision, error) {
	return ToolAuthorizationDecision{}, nil
}

func (allowInteractionTools) ResolveToolApproval(context.Context, ToolAuthorizationRequest, runs.ApprovalPrompt, interrupt.Resolution) (ToolAuthorizationDecision, error) {
	return ToolAuthorizationDecision{}, nil
}

type denyingInteractionTools struct{ reason string }

func (authorizer denyingInteractionTools) AuthorizeTool(context.Context, ToolAuthorizationRequest) (ToolAuthorizationDecision, error) {
	return ToolAuthorizationDecision{Denied: true, Reason: authorizer.reason}, nil
}

func (authorizer denyingInteractionTools) ResolveToolApproval(context.Context, ToolAuthorizationRequest, runs.ApprovalPrompt, interrupt.Resolution) (ToolAuthorizationDecision, error) {
	return ToolAuthorizationDecision{Denied: true, Reason: authorizer.reason}, nil
}

type testInteractionToolInterpreter struct{}

func (testInteractionToolInterpreter) SafetyClass(string) domaintool.SafetyClass {
	return domaintool.SafetyClassSafe
}

func (testInteractionToolInterpreter) UsesStandardPolicy(string) bool { return true }

func (testInteractionToolInterpreter) ApprovalSubject(string, domaintool.Arguments) (string, error) {
	return "", nil
}

func (testInteractionToolInterpreter) ShellCommand(string, string) string { return "" }

func (testInteractionToolInterpreter) ProjectOutcome(context.Context, string, string, bool) (runs.ExecutionFact, error) {
	return nil, nil
}

type failingOutcomeInteractionInterpreter struct{ testInteractionToolInterpreter }

func (failingOutcomeInteractionInterpreter) ProjectOutcome(context.Context, string, string, bool) (runs.ExecutionFact, error) {
	return nil, errors.New("projection unavailable")
}

type testInteractionToolPresenter struct{}

func (testInteractionToolPresenter) Activity(string, domaintool.Arguments) string {
	return "Echoing value"
}

func (testInteractionToolPresenter) Present(_ string, _ domaintool.Arguments, result domaintool.Result) (domaintool.Result, string) {
	return result, "presented"
}

type recordingInteractionHooks struct {
	before int
	after  int
}

type failingAfterInteractionHooks struct{ after int }

func (*failingAfterInteractionHooks) BeforeToolUse(context.Context, InteractionToolHookInput) (InteractionToolHookDecision, error) {
	return InteractionToolHookDecision{}, nil
}

func (hooks *failingAfterInteractionHooks) AfterToolUse(context.Context, InteractionToolHookInput) error {
	hooks.after++
	return errors.New("post-Tool hook unavailable")
}

func (hooks *recordingInteractionHooks) BeforeToolUse(context.Context, InteractionToolHookInput) (InteractionToolHookDecision, error) {
	hooks.before++
	return InteractionToolHookDecision{}, nil
}

func (hooks *recordingInteractionHooks) AfterToolUse(context.Context, InteractionToolHookInput) error {
	hooks.after++
	return nil
}

type observationScriptModel struct {
	mu        sync.Mutex
	responses []*chat.Response
}

type streamingObservationModel struct {
	chunks   int
	streamed chan struct{}
}

func (streamingObservationModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return nil, errors.New("unexpected synchronous model call")
}

func (model streamingObservationModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) {
		defer close(model.streamed)
		for index := range model.chunks {
			message := chat.NewAssistantMessage(chat.NewTextPart("x"))
			response := &chat.Response{Choices: []chat.Choice{{Index: 0, Message: &message}}}
			if index == model.chunks-1 {
				response.Model = "test-model"
				response.Usage = chat.Usage{InputTokens: 5, OutputTokens: 2}
				response.Choices[0].FinishReason = chat.FinishReasonStop
			}
			if !yield(response, nil) {
				return
			}
		}
	}
}

func (model *observationScriptModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	if len(model.responses) == 0 {
		return nil, errors.New("unexpected model call")
	}
	response := model.responses[0]
	model.responses = model.responses[1:]
	return response.Clone(), nil
}

type manifestScriptModel struct {
	mu        sync.Mutex
	call      int
	manifests [][]string
}

type doomLoopScriptModel struct {
	mu   sync.Mutex
	call int
}

func (model *doomLoopScriptModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.call++
	if model.call <= interactionDoomLoopThreshold+1 {
		return interactionToolResponse(chat.ToolCall{
			ID: "lookup_" + strconv.Itoa(model.call), Name: "lookup", Arguments: `{}`,
		}, 1, 1), nil
	}
	return interactionUsageTextResponse("changed approach", 1, 1), nil
}

func (model *manifestScriptModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	names := make([]string, len(request.Tools))
	for index, definition := range request.Tools {
		names[index] = definition.Name
	}
	model.manifests = append(model.manifests, names)
	model.call++
	switch model.call {
	case 1:
		return interactionToolResponse(chat.ToolCall{
			ID: "discover", Name: "search_tools", Arguments: `{"query":"select:hidden_lookup"}`,
		}, 1, 1), nil
	case 2:
		return interactionToolResponse(chat.ToolCall{
			ID: "lookup", Name: "hidden_lookup", Arguments: `{}`,
		}, 1, 1), nil
	default:
		return interactionUsageTextResponse("done", 1, 1), nil
	}
}

func newObservedTestInteractionExecutor(
	t *testing.T,
	model chat.Model,
	extra InteractionExecutorConfig,
) *InteractionExecutor {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	extra.DefaultClient = client
	extra.ImplementationIdentity = "interaction-observation-test-build"
	extra.ConfigurationIdentity = "interaction-observation-test-config"
	extra.BuildID = interactionTestBuildID
	extra.DefaultMaxModelCalls = 8
	extra.UnknownEffectPollInterval = 5 * time.Millisecond
	executor, err := NewInteractionExecutor(extra)
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func runInteractionHarnessWithCommit(
	t *testing.T,
	executor *InteractionExecutor,
	start runs.RootExecutionStart,
	commitFact func(runs.ExecutionFact) error,
) []runs.ExecutorEvent {
	t.Helper()
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := executor.Observe(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	eventsReady := make(chan []runs.ExecutorEvent, 1)
	go func() {
		var events []runs.ExecutorEvent
		for event := range sequence {
			if commit, authoritative := event.Payload.(runs.ExecutionFactCommit); authoritative {
				commit.Complete(commitFact(commit.Fact))
				event.Payload = commit.Fact
			}
			events = append(events, event)
			if _, unknown := event.Payload.(runs.UnknownEffectsDetected); unknown {
				break
			}
		}
		eventsReady <- events
	}()
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	events := <-eventsReady
	if err := executor.Release(context.Background(), ref); err != nil {
		t.Fatal(err)
	}
	return events
}

func interactionToolResponse(call chat.ToolCall, inputTokens, outputTokens int64) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
	return &chat.Response{
		Model: "test-model", Usage: chat.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
		Choices: []chat.Choice{{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls}},
	}
}

func interactionToolBatchResponse(calls []chat.ToolCall, inputTokens, outputTokens int64) *chat.Response {
	parts := make([]chat.Part, len(calls))
	for index, call := range calls {
		parts[index] = chat.NewToolCallPart(call)
	}
	message := chat.NewAssistantMessage(parts...)
	return &chat.Response{
		Model: "test-model", Usage: chat.Usage{InputTokens: inputTokens, OutputTokens: outputTokens},
		Choices: []chat.Choice{{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls}},
	}
}

func interactionUsageTextResponse(text string, inputTokens, outputTokens int64) *chat.Response {
	response := interactionTextResponse(text)
	response.Model = "test-model"
	response.Usage = chat.Usage{InputTokens: inputTokens, OutputTokens: outputTokens}
	return response
}
