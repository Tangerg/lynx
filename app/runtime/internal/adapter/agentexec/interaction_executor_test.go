package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

const interactionTestBuildID = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestInteractionExecutorRequiresProcessLifetime(t *testing.T) {
	client, err := chatclient.New(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return interactionTextResponse("unused"), nil
	}), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		DefaultClient:          client,
		DefaultSelection:       testDefaultSelection(),
		ImplementationIdentity: "interaction-executor-test-build",
		ConfigurationIdentity:  "interaction-executor-test-config",
		BuildID:                interactionTestBuildID,
	})
	if err == nil || executor != nil {
		t.Fatalf("NewInteractionExecutor without lifetime = (%v, %v), want nil executor and non-nil error", executor, err)
	}
}

func TestInteractionExecutorRequiresExactDefaultIdentity(t *testing.T) {
	client, err := chatclient.New(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return interactionTextResponse("unused"), nil
	}), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:               t.Context(),
		DefaultClient:          client,
		ImplementationIdentity: "interaction-executor-test-build",
		ConfigurationIdentity:  "interaction-executor-test-config",
		BuildID:                interactionTestBuildID,
	})
	if err == nil || executor != nil {
		t.Fatalf("NewInteractionExecutor without default selection = (%v, %v), want nil executor and non-nil error", executor, err)
	}
}

func TestInteractionExecutorResolvesDefaultThroughResolverWithoutImplicitSelection(t *testing.T) {
	client, err := chatclient.New(chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return interactionTextResponse("unused"), nil
	}), chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	var resolved modelref.Selection
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime: t.Context(), DefaultSelection: testDefaultSelection(),
		ChatResolver: interactionChatResolverFunc(func(_ context.Context, selection modelref.Selection) (*chatclient.Client, error) {
			resolved = selection
			return client, nil
		}),
		ImplementationIdentity: "interaction-executor-test-build",
		ConfigurationIdentity:  "interaction-executor-test-config",
		BuildID:                interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := executor.resolveClient(t.Context(), testDefaultSelection())
	if err != nil || got != client || resolved != testDefaultSelection() {
		t.Fatalf("resolve exact default = (%p, %v, %#v), want (%p, nil, %#v)", got, err, resolved, client, testDefaultSelection())
	}
	if _, err := executor.resolveClient(t.Context(), modelref.Selection{}); err == nil {
		t.Fatal("resolve implicit selection succeeded, want exact-selection error")
	}
}

type interactionChatResolverFunc func(context.Context, modelref.Selection) (*chatclient.Client, error)

func (resolve interactionChatResolverFunc) ResolveChat(
	ctx context.Context,
	selection modelref.Selection,
) (*chatclient.Client, error) {
	return resolve(ctx, selection)
}

func TestInteractionExecutorRunsRootFromCompleteWorkingContext(t *testing.T) {
	var captured []chat.Message
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		captured = cloneChatMessages(request.Messages)
		return interactionTextResponse("complete answer"), nil
	})
	executor := newTestInteractionExecutor(t, model)
	start := interactionTestStart()
	start.WorkingContext = []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("earlier question")),
		chat.NewAssistantMessage(chat.NewTextPart("earlier answer")),
		chat.NewUserMessage(chat.NewTextPart("current question")),
	}

	events := runInteractionHarness(context.Background(), t, executor, start, nil)
	if len(captured) != 3 || captured[0].Text() != "earlier question" || captured[2].Text() != "current question" {
		t.Fatalf("model working context = %#v", captured)
	}
	if deltas := payloadsOf[runs.MessageDelta](events); len(deltas) != 0 {
		t.Fatalf("non-streaming execution emitted message deltas: %#v", deltas)
	}
	completed := payloadsOf[runs.AssistantMessageCompleted](events)
	if len(completed) != 1 || completed[0].Message.Text() != "complete answer" {
		t.Fatalf("authoritative assistant completion = %#v", completed)
	}
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted || ended[0].Usage == nil || ended[0].Usage.Steps != 1 {
		t.Fatalf("segment end = %#v", ended)
	}
	assertOneRootMember(t, events)
}

func TestInteractionExecutorStagesWithoutCallingModel(t *testing.T) {
	var calls atomic.Int64
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return interactionTextResponse("unexpected"), nil
	})
	executor := newTestInteractionExecutor(t, model)
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("model calls during stage = %d", calls.Load())
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatalf("second release: %v", err)
	}
}

func TestInteractionExecutorMapsModelFailure(t *testing.T) {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("provider unavailable")
	})
	executor := newTestInteractionExecutor(t, model)
	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), nil)
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeFailed || ended[0].Failure == nil ||
		ended[0].Failure.Kind != run.FailureProviderUnavailable {
		t.Fatalf("segment end = %#v", ended)
	}
}

func TestInteractionExecutorDoesNotBindRunLifetimeToCallerContext(t *testing.T) {
	modelContext := make(chan context.Context, 1)
	release := make(chan struct{})
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		modelContext <- ctx
		<-release
		return interactionTextResponse("caller returned; Run continued"), nil
	})
	executor := newTestInteractionExecutor(t, model)
	caller, cancelCaller := context.WithCancel(context.Background())
	events := runInteractionHarness(caller, t, executor, interactionTestStart(), func() {
		runContext := <-modelContext
		cancelCaller()
		if err := runContext.Err(); err != nil {
			t.Fatalf("Run lifecycle context followed caller cancellation: %v", err)
		}
		close(release)
	})
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCompleted {
		t.Fatalf("segment end = %#v", ended)
	}
}

func TestInteractionExecutorBindsRunLifetimeToProcessOwner(t *testing.T) {
	lifetime, stopLifetime := context.WithCancel(context.Background())
	modelContext := make(chan context.Context, 1)
	model := chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		modelContext <- ctx
		<-ctx.Done()
		return nil, ctx.Err()
	})
	executor := newTestInteractionExecutorWithLifetime(t, lifetime, model)
	events := runInteractionHarness(context.Background(), t, executor, interactionTestStart(), func() {
		runContext := <-modelContext
		stopLifetime()
		select {
		case <-runContext.Done():
		case <-time.After(time.Second):
			t.Fatal("process lifetime cancellation did not reach the Run")
		}
	})
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("segment end after process cancellation = %#v", ended)
	}
}

func TestInteractionExecutorReportsDispatcherPanicAsUnknownEffect(t *testing.T) {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		panic("provider panic")
	})
	executor := newTestInteractionExecutor(t, model)
	events := runInteractionHarnessWithCommit(
		t, executor, interactionTestStart(), func(runs.ExecutionFact) error { return nil },
	)
	unknown := payloadsOf[runs.UnknownEffectsDetected](events)
	if len(unknown) != 1 || len(unknown[0].IDs) != 1 {
		t.Fatalf("panic unknown effects = %#v", unknown)
	}
	if ended := payloadsOf[runs.SegmentEnded](events); len(ended) != 0 {
		t.Fatalf("dispatcher panic was projected as a definite terminal = %#v", ended)
	}
}

func TestInteractionTerminationMappingIsComplete(t *testing.T) {
	tests := []struct {
		name, status, cause, reason string
		failureKind, failureCode    string
		wantOutcome                 run.Outcome
		wantFailure                 run.FailureKind
		hasFailure                  bool
	}{
		{name: "completion", status: "completed", cause: "completion", wantOutcome: run.OutcomeCompleted},
		{name: "process deadline", status: "timed_out", cause: "process_deadline", reason: "process deadline", wantOutcome: run.OutcomeTimedOut, wantFailure: run.FailureTimeout, hasFailure: true},
		{name: "parent deadline", status: "timed_out", cause: "parent_deadline", reason: "parent deadline", wantOutcome: run.OutcomeTimedOut, wantFailure: run.FailureTimeout, hasFailure: true},
		{name: "host deadline", status: "timed_out", cause: "host_deadline", reason: "host deadline", wantOutcome: run.OutcomeTimedOut, wantFailure: run.FailureTimeout, hasFailure: true},
		{name: "parent cancellation", status: "canceled", cause: "parent_cancellation", reason: "parent canceled", wantOutcome: run.OutcomeCanceled},
		{name: "host cancellation", status: "canceled", cause: "host_cancellation", reason: "host canceled", wantOutcome: run.OutcomeCanceled},
		{name: "model call limit", status: "failed", cause: "execution_failure", reason: "model call limit", failureKind: "execution", failureCode: "interaction.limit.model_calls", wantOutcome: run.OutcomeMaxSteps},
		{name: "strategy failure", status: "failed", cause: "execution_failure", reason: "strategy failed", failureKind: "execution", failureCode: "execution.failed", wantOutcome: run.OutcomeFailed, wantFailure: run.FailureAgentStuck, hasFailure: true},
		{name: "external failure", status: "failed", cause: "external_failure", reason: "provider unavailable", failureKind: "external", failureCode: "provider.failed", wantOutcome: run.OutcomeFailed, wantFailure: run.FailureProviderUnavailable, hasFailure: true},
		{name: "host projection failure", status: "failed", cause: "external_failure", reason: "journal unavailable", failureKind: "external", failureCode: "interaction.host.failed", wantOutcome: run.OutcomeFailed, wantFailure: run.FailureInternal, hasFailure: true},
		{name: "contract failure", status: "failed", cause: "contract_failure", reason: "contract failed", failureKind: "contract", failureCode: "contract.failed", wantOutcome: run.OutcomeFailed, wantFailure: run.FailureInternal, hasFailure: true},
		{name: "panic", status: "failed", cause: "panic", reason: "execution panicked", failureKind: "panic", failureCode: "execution.panic", wantOutcome: run.OutcomeFailed, wantFailure: run.FailureInternal, hasFailure: true},
		{name: "engine kill", status: "killed", cause: "engine_kill", reason: "engine shutdown", wantOutcome: run.OutcomeFailed, wantFailure: run.FailureInternal, hasFailure: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wire := map[string]any{
				"status": test.status, "cause": test.cause, "reason": test.reason,
			}
			if test.failureKind != "" {
				wire["failure"] = map[string]string{
					"kind": test.failureKind, "code": test.failureCode, "message": test.reason,
				}
			}
			payload, err := json.Marshal(wire)
			if err != nil {
				t.Fatal(err)
			}
			var termination agent.Termination
			if err := json.Unmarshal(payload, &termination); err != nil {
				t.Fatal(err)
			}
			end := segmentEndFromTermination(termination, time.Second)
			if end.Reason != test.wantOutcome || (end.Failure != nil) != test.hasFailure {
				t.Fatalf("mapping = %#v", end)
			}
			if test.hasFailure && end.Failure.Kind != test.wantFailure {
				t.Fatalf("failure = %#v, want kind %v", end.Failure, test.wantFailure)
			}
		})
	}
}

func TestInteractionSegmentDurationExcludesTimeBeforeContinuation(t *testing.T) {
	processStartedAt := time.Date(2026, 8, 11, 1, 0, 0, 0, time.UTC)
	continuedAt := processStartedAt.Add(8 * time.Hour)
	finishedAt := continuedAt.Add(750 * time.Millisecond)

	if got := interactionSegmentDuration(processStartedAt, continuedAt, finishedAt); got != 750*time.Millisecond {
		t.Fatalf("resumed Segment duration = %v, want 750ms", got)
	}
	if got := interactionSegmentDuration(processStartedAt, time.Time{}, finishedAt); got != 8*time.Hour+750*time.Millisecond {
		t.Fatalf("initial Segment duration = %v, want full Process lifetime", got)
	}
	if got := interactionSegmentDuration(processStartedAt, finishedAt.Add(time.Second), finishedAt); got != 0 {
		t.Fatalf("clock-regressed Segment duration = %v, want 0", got)
	}
}

func newTestInteractionExecutor(t *testing.T, model chat.Model) *InteractionExecutor {
	return newTestInteractionExecutorWithLifetime(t, t.Context(), model)
}

func newTestInteractionExecutorWithLifetime(
	t *testing.T,
	lifetime context.Context,
	model chat.Model,
) *InteractionExecutor {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		Lifetime:      lifetime,
		DefaultClient: client, DefaultSelection: testDefaultSelection(), ImplementationIdentity: "interaction-executor-test-build",
		ConfigurationIdentity: "interaction-executor-test-config", DefaultMaxModelCalls: 4,
		BuildID: interactionTestBuildID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func interactionTestStart() runs.RootExecutionStart {
	return runs.RootExecutionStart{
		SessionID: "session_1", Message: "current question", ModelSelection: testDefaultSelection(),
		WorkingContext: []chat.Message{chat.NewUserMessage(chat.NewTextPart("current question"))},
	}
}

func runInteractionHarness(
	ctx context.Context,
	t *testing.T,
	executor *InteractionExecutor,
	start runs.RootExecutionStart,
	afterBegin func(),
) []runs.ExecutorEvent {
	t.Helper()
	ref, err := executor.StageRoot(ctx, start)
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
	if err := executor.BeginRoot(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if afterBegin != nil {
		afterBegin()
	}
	return <-eventsReady
}

func payloadsOf[T any](events []runs.ExecutorEvent) []T {
	return slices.Collect(func(yield func(T) bool) {
		for _, event := range events {
			if payload, ok := event.Payload.(T); ok && !yield(payload) {
				return
			}
		}
	})
}

func assertOneRootMember(t *testing.T, events []runs.ExecutorEvent) {
	t.Helper()
	members := make(map[string]struct{})
	for _, event := range events {
		if event.Member.MemberID == "" || event.Member.Child() {
			t.Fatalf("event member = %#v", event.Member)
		}
		members[event.Member.MemberID] = struct{}{}
	}
	if len(members) != 1 {
		t.Fatalf("root member identities = %v", members)
	}
}

func interactionTextResponse(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Result: &chat.Result{Message: &message, FinishReason: chat.FinishReasonStop}}
}
