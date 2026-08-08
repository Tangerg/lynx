package agentexec

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
)

func TestInteractionExecutorRunsNativeRootFromCompleteWorkingContext(t *testing.T) {
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

	events := runInteractionHarness(t, executor, context.Background(), start, nil)
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
	events := runInteractionHarness(t, executor, context.Background(), interactionTestStart(), nil)
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeFailed || ended[0].Problem == nil ||
		ended[0].Problem.Kind != transcript.ProviderUnavailableProblem {
		t.Fatalf("segment end = %#v", ended)
	}
}

func TestInteractionExecutorMapsHostIntentRatherThanModelError(t *testing.T) {
	for _, test := range []struct {
		name    string
		hostErr error
		outcome run.Outcome
		problem transcript.ProblemKind
	}{
		{name: "cancel", hostErr: context.Canceled, outcome: run.OutcomeCanceled},
		{name: "deadline", hostErr: context.DeadlineExceeded, outcome: run.OutcomeTimedOut, problem: transcript.TimeoutProblem},
	} {
		t.Run(test.name, func(t *testing.T) {
			entered := make(chan struct{})
			release := make(chan struct{})
			model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
				close(entered)
				<-release
				return nil, errors.New("model call also failed")
			})
			executor := newTestInteractionExecutor(t, model)
			host := newControlledContext(test.hostErr)
			events := runInteractionHarness(t, executor, host, interactionTestStart(), func() {
				<-entered
				host.Trigger()
				close(release)
			})
			ended := payloadsOf[runs.SegmentEnded](events)
			if len(ended) != 1 || ended[0].Reason != test.outcome {
				t.Fatalf("segment end = %#v", ended)
			}
			if test.outcome == run.OutcomeTimedOut && (ended[0].Problem == nil || ended[0].Problem.Kind != test.problem) {
				t.Fatalf("timeout problem = %#v", ended[0].Problem)
			}
		})
	}
}

func TestInteractionExecutorIsolatesDispatcherPanic(t *testing.T) {
	entered := make(chan struct{})
	host := newControlledContext(context.Canceled)
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		close(entered)
		panic("provider panic")
	})
	executor := newTestInteractionExecutor(t, model)
	events := runInteractionHarness(t, executor, host, interactionTestStart(), func() {
		<-entered
		host.Trigger()
	})
	ended := payloadsOf[runs.SegmentEnded](events)
	if len(ended) != 1 || ended[0].Reason != run.OutcomeCanceled {
		t.Fatalf("panic-isolated terminal = %#v", ended)
	}
}

func TestInteractionTerminationMappingIncludesFrameworkPanic(t *testing.T) {
	failure := `{"kind":"panic","code":"execution.panic","message":"execution panicked"}`
	var termination agent.Termination
	payload := `{"status":"failed","cause":"panic","reason":"execution panicked","failure":` + failure + `}`
	if err := json.Unmarshal([]byte(payload), &termination); err != nil {
		t.Fatal(err)
	}
	end := segmentEndFromTermination(termination, time.Second)
	if end.Reason != run.OutcomeFailed || end.Problem == nil || end.Problem.Kind != transcript.InternalProblem {
		t.Fatalf("panic mapping = %#v", end)
	}
}

func newTestInteractionExecutor(t *testing.T, model chat.Model) *InteractionExecutor {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	executor, err := NewInteractionExecutor(InteractionExecutorConfig{
		DefaultClient: client, ImplementationIdentity: "interaction-executor-test-build",
		ConfigurationIdentity: "interaction-executor-test-config", DefaultMaxModelCalls: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	return executor
}

func interactionTestStart() runs.RootExecutionStart {
	return runs.RootExecutionStart{
		SessionID: "session_1", Message: "current question",
		WorkingContext: []chat.Message{chat.NewUserMessage(chat.NewTextPart("current question"))},
	}
}

func runInteractionHarness(
	t *testing.T,
	executor *InteractionExecutor,
	ctx context.Context,
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
	return &chat.Response{Choices: []chat.Choice{{
		Index: 0, Message: &message, FinishReason: chat.FinishReasonStop,
	}}}
}

type controlledContext struct {
	context.Context
	done chan struct{}
	err  error
	once sync.Once
}

func newControlledContext(err error) *controlledContext {
	return &controlledContext{Context: context.Background(), done: make(chan struct{}), err: err}
}

func (ctx *controlledContext) Done() <-chan struct{} { return ctx.done }

func (ctx *controlledContext) Err() error {
	select {
	case <-ctx.done:
		return ctx.err
	default:
		return nil
	}
}

func (ctx *controlledContext) Trigger() { ctx.once.Do(func() { close(ctx.done) }) }
