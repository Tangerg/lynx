package runtime_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/hitl"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type managedModel struct {
	mu    sync.Mutex
	calls int
}

func (m *managedModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls == 1 {
		message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "approval", Arguments: `{}`}))
		response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls})
		response.Model = "managed-model"
		response.Usage = chat.Usage{InputTokens: 5, OutputTokens: 2}
		return response, err
	}
	if len(request.Messages) != 3 || request.Messages[2].Role != chat.RoleTool {
		panic("managed interaction did not continue with tool results")
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("complete"))
	response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	response.Model = "managed-model"
	response.Usage = chat.Usage{InputTokens: 8, OutputTokens: 3}
	return response, err
}

func (m *managedModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type managedConcurrentTool struct {
	name string
	call func(context.Context) (string, error)
}

func (t *managedConcurrentTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        t.name,
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}
}

func (t *managedConcurrentTool) Call(ctx context.Context, _ string) (string, error) {
	return t.call(ctx)
}

func (t *managedConcurrentTool) ConcurrencyKey(string) (string, bool) {
	return "", true
}

type managedConcurrentModel struct {
	mu    sync.Mutex
	calls int
}

func (m *managedConcurrentModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls == 1 {
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "call-1", Name: "first", Arguments: `{}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "call-2", Name: "second", Arguments: `{}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "call-3", Name: "third", Arguments: `{}`}),
		)
		return chat.NewResponse(chat.Choice{
			Index:        0,
			Message:      &message,
			FinishReason: chat.FinishReasonToolCalls,
		})
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != chat.RoleTool ||
		len(last.Parts) != 3 ||
		last.Parts[0].ToolResult.ID != "call-1" ||
		last.Parts[1].ToolResult.ID != "call-2" ||
		last.Parts[2].ToolResult.ID != "call-3" {
		return nil, errors.New("managed interaction committed tool results out of model-call order")
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("complete"))
	return chat.NewResponse(chat.Choice{
		Index:        0,
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	})
}

func TestManagedInteractionPublishesOwnedBoundariesAndRecordsUsage(t *testing.T) {
	model := &managedModel{}
	tool, err := tools.New[struct{}, string](tools.Config{Name: "approval"}, func(context.Context, struct{}) (string, error) {
		return "approved", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tools.NewRegistry(tool)
	if err != nil {
		t.Fatal(err)
	}

	var boundaries []event.InteractionBoundary
	listener := event.NewNamedListener("managed-boundaries", func(_ context.Context, value event.Event) {
		if boundary, ok := value.(event.InteractionBoundary); ok {
			boundaries = append(boundaries, boundary)
		}
	})
	a := managedInteractionAgent(t, "managed-events", model, registry, interaction.Limits{})
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{listener}})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls() != 2 || len(proc.ModelCalls()) != 2 {
		t.Fatalf("model calls = %d, invocations = %d", model.Calls(), len(proc.ModelCalls()))
	}
	if len(boundaries) != 6 {
		t.Fatalf("interaction boundaries = %d, want 6", len(boundaries))
	}
	for _, boundary := range boundaries {
		if boundary.ProcessID() != proc.ID() || boundary.Deployment != proc.Deployment() || boundary.InteractionID == "" {
			t.Fatalf("unowned boundary = %#v", boundary)
		}
	}
	if boundaries[0].Boundary.Round != 1 || boundaries[4].Boundary.Round != 2 {
		t.Fatalf("rounds = %d, %d", boundaries[0].Boundary.Round, boundaries[4].Boundary.Round)
	}
}

func TestManagedInteractionHonorsConcurrentToolCallLimit(t *testing.T) {
	started := make(chan string, 3)
	releases := map[string]chan struct{}{
		"first":  make(chan struct{}),
		"second": make(chan struct{}),
		"third":  make(chan struct{}),
	}
	registered := make([]tools.Tool, 0, len(releases))
	for _, name := range []string{"first", "second", "third"} {
		registered = append(registered, &managedConcurrentTool{
			name: name,
			call: func(context.Context) (string, error) {
				started <- name
				<-releases[name]
				return name, nil
			},
		})
	}
	registry, err := tools.NewRegistry(registered...)
	if err != nil {
		t.Fatal(err)
	}
	model := &managedConcurrentModel{}
	a := managedInteractionAgent(t, "managed-concurrency-limit", model, registry, interaction.Limits{
		MaxConcurrentToolCalls: 2,
	})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)

	proc, done := engine.Start(t.Context(), a, managedInput(), core.ProcessOptions{})
	firstStarted := <-started
	secondStarted := <-started
	select {
	case thirdStarted := <-started:
		t.Fatalf("third tool %q started before a concurrency slot was released", thirdStarted)
	default:
	}

	close(releases[firstStarted])
	thirdStarted := <-started
	close(releases[secondStarted])
	close(releases[thirdStarted])
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusCompleted || proc.Failure() != nil {
		t.Fatalf("process status=%s failure=%v", proc.Status(), proc.Failure())
	}
}

func TestManagedInteractionRejectsNegativeConcurrentToolCallLimit(t *testing.T) {
	model := &managedFinalModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	a := managedInteractionAgent(t, "managed-negative-concurrency", model, registry, interaction.Limits{
		MaxConcurrentToolCalls: -1,
	})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if proc.Failure() == nil || model.Calls() != 0 {
		t.Fatalf("failure=%v model calls=%d", proc.Failure(), model.Calls())
	}
}

func TestManagedInteractionSuspendsAndResumesPendingToolExactly(t *testing.T) {
	model := &managedModel{}
	var attempts int
	tool, err := tools.New[struct{}, string](tools.Config{Name: "approval"}, func(ctx context.Context, _ struct{}) (string, error) {
		attempts++
		approved, err := hitl.Interrupt[bool](ctx, "approval-1", map[string]any{"message": "approve?"})
		if err != nil {
			return "", err
		}
		if !approved {
			return "denied", nil
		}
		return "approved", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, _ := tools.NewRegistry(tool)
	a := managedInteractionAgent(t, "managed-resume", model, registry, interaction.Limits{})
	var boundaries []interaction.EventKind
	listener := event.NewNamedListener("managed-resume-boundaries", func(_ context.Context, value event.Event) {
		if boundary, ok := value.(event.InteractionBoundary); ok {
			boundaries = append(boundaries, boundary.Boundary.Kind)
		}
	})
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{listener}})
	mustDeploy(t, engine, a)

	proc, done := engine.Start(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusWaiting || proc.Suspension() == nil || proc.Suspension().Kind != interaction.SuspensionTool {
		t.Fatalf("parked process = status %s suspension %#v", proc.Status(), proc.Suspension())
	}
	snapshot, err := proc.Snapshot()
	if err != nil {
		t.Fatalf("snapshot waiting interaction: %v", err)
	}
	if err := runtime.ValidateResumableSnapshot(snapshot); err != nil {
		t.Fatalf("ValidateResumableSnapshot: %v", err)
	}
	invalid := snapshot
	invalid.Suspension = snapshot.Suspension.Clone()
	invalid.Suspension.Payload = json.RawMessage(`{}`)
	if err := runtime.ValidateResumableSnapshot(invalid); err == nil {
		t.Fatal("ValidateResumableSnapshot accepted a malformed managed checkpoint")
	}
	if model.Calls() != 1 || attempts != 1 {
		t.Fatalf("before resume model=%d tool=%d", model.Calls(), attempts)
	}
	if err := engine.Resume(proc.ID(), "approval-1", true); err != nil {
		t.Fatal(err)
	}
	if err := engine.Continue(t.Context(), proc.ID()); err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusCompleted || model.Calls() != 2 || attempts != 2 {
		t.Fatalf("after resume status=%s model=%d tool=%d failure=%v", proc.Status(), model.Calls(), attempts, proc.Failure())
	}
	if proc.Suspension() != nil {
		t.Fatalf("completed process retained suspension %#v", proc.Suspension())
	}
	wantBoundaries := []interaction.EventKind{
		interaction.EventModelRequest,
		interaction.EventModelResponse,
		interaction.EventToolCall,
		interaction.EventPause,
		interaction.EventResume,
		interaction.EventToolCall,
		interaction.EventToolResult,
		interaction.EventModelRequest,
		interaction.EventModelResponse,
	}
	if len(boundaries) != len(wantBoundaries) {
		t.Fatalf("resume boundaries = %v, want %v", boundaries, wantBoundaries)
	}
	for i := range wantBoundaries {
		if boundaries[i] != wantBoundaries[i] {
			t.Fatalf("resume boundaries = %v, want %v", boundaries, wantBoundaries)
		}
	}
}

func TestManagedInteractionStopsBeforeContinuationAtStepLimit(t *testing.T) {
	model := &managedModel{}
	tool, _ := tools.New[struct{}, string](tools.Config{Name: "approval"}, func(context.Context, struct{}) (string, error) { return "ok", nil })
	registry, _ := tools.NewRegistry(tool)
	a := managedInteractionAgent(t, "managed-steps", model, registry, interaction.Limits{MaxSteps: 1})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls() != 1 || proc.Status() != core.StatusCompleted {
		t.Fatalf("model calls=%d status=%s", model.Calls(), proc.Status())
	}
}

func TestManagedInteractionStopsBeforeContinuationAtModelCallLimit(t *testing.T) {
	model := &managedModel{}
	tool, _ := tools.New[struct{}, string](tools.Config{Name: "approval"}, func(context.Context, struct{}) (string, error) { return "ok", nil })
	registry, _ := tools.NewRegistry(tool)
	a := managedInteractionAgent(t, "managed-model-calls", model, registry, interaction.Limits{MaxModelCalls: 1})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls() != 1 || len(proc.ModelCalls()) != 1 || proc.Status() != core.StatusCompleted {
		t.Fatalf("provider calls=%d recorded calls=%d status=%s", model.Calls(), len(proc.ModelCalls()), proc.Status())
	}
}

type managedFinalModel struct {
	mu    sync.Mutex
	calls int
}

func (m *managedFinalModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	message := chat.NewAssistantMessage(chat.NewTextPart("committed"))
	response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	response.Model = "managed-final"
	return response, err
}

func (m *managedFinalModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestManagedInteractionDoesNotRetryActionAfterCommittedBoundary(t *testing.T) {
	model := &managedFinalModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	observerErr := errors.New("observer stopped")
	a := agent.New(agent.AgentConfig{Name: "managed-committed", Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("run")))
		if err != nil {
			return "", err
		}
		_, err = pc.Interact(ctx, core.Interaction{Model: model, Request: request, Tools: registry, Observe: func(_ context.Context, boundary interaction.Event) error {
			if boundary.Kind == interaction.EventModelResponse {
				return observerErr
			}
			return nil
		}})
		return "", err
	}, core.ActionConfig{Retry: core.RetryPolicy{MaxAttempts: 3, Safety: core.RetrySafetyIdempotent}})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run control-flow error = %v", err)
	}
	if proc == nil || !errors.Is(proc.Failure(), interaction.ErrCommitted) || !errors.Is(proc.Failure(), observerErr) {
		t.Fatalf("process failure = %v", proc.Failure())
	}
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want 1 despite MaxAttempts=3", model.Calls())
	}
}

type managedCrashModel struct {
	mu    sync.Mutex
	calls int
}

func (m *managedCrashModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls == 1 {
		message := chat.NewAssistantMessage(
			chat.NewToolCallPart(chat.ToolCall{ID: "first-1", Name: "first", Arguments: `{}`}),
			chat.NewToolCallPart(chat.ToolCall{ID: "approval-1", Name: "approval", Arguments: `{}`}),
		)
		response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls})
		response.Model = "managed-crash"
		return response, err
	}
	last := request.Messages[len(request.Messages)-1]
	if last.Role != chat.RoleTool || len(last.Parts) != 2 || last.Parts[0].ToolResult.Result != "first done" || last.Parts[1].ToolResult.Result != "approved" {
		panic("managed checkpoint did not reconstruct both tool results")
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("complete"))
	response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	response.Model = "managed-crash"
	return response, err
}

func (m *managedCrashModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestManagedInteractionRestoresAfterCrashWithoutReplayingCommittedWork(t *testing.T) {
	model := &managedCrashModel{}
	completedCalls := 0
	approvalAttempts := 0
	first, err := tools.New[struct{}, string](tools.Config{Name: "first"}, func(context.Context, struct{}) (string, error) {
		completedCalls++
		return "first done", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	approval, err := tools.New[struct{}, string](tools.Config{Name: "approval"}, func(ctx context.Context, _ struct{}) (string, error) {
		approvalAttempts++
		approved, err := hitl.Interrupt[bool](ctx, "approval-crash", map[string]any{"message": "approve?"})
		if err != nil {
			return "", err
		}
		if !approved {
			return "denied", nil
		}
		return "approved", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := tools.NewRegistry(first, approval)
	if err != nil {
		t.Fatal(err)
	}
	a := managedInteractionAgent(t, "managed-crash-restore", model, registry, interaction.Limits{})
	const buildID = "managed-crash-fixture"
	engine1 := agent.MustNewEngine(runtime.Config{BuildID: buildID})
	mustDeploy(t, engine1, a)

	proc, done := engine1.Start(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusWaiting || model.Calls() != 1 || completedCalls != 1 || approvalAttempts != 1 {
		t.Fatalf("before crash status=%s model=%d completed=%d approval=%d", proc.Status(), model.Calls(), completedCalls, approvalAttempts)
	}

	captured, err := proc.Snapshot()
	if err != nil {
		t.Fatalf("capture crash snapshot: %v", err)
	}
	body, err := json.Marshal(captured)
	if err != nil {
		t.Fatalf("marshal crash snapshot: %v", err)
	}
	var snapshot core.ProcessSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("unmarshal crash snapshot: %v", err)
	}
	engine2 := agent.MustNewEngine(runtime.Config{BuildID: buildID})
	mustDeploy(t, engine2, a)
	restored, err := engine2.RestoreSnapshot(snapshot, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("restore after crash: %v", err)
	}
	if err := engine2.Resume(restored.ID(), "approval-crash", true); err != nil {
		t.Fatalf("resume after crash: %v", err)
	}
	if err := engine2.Continue(t.Context(), restored.ID()); err != nil {
		t.Fatalf("continue after crash: %v", err)
	}
	if restored.Status() != core.StatusCompleted || model.Calls() != 2 || completedCalls != 1 || approvalAttempts != 2 {
		t.Fatalf("after crash status=%s model=%d completed=%d approval=%d failure=%v", restored.Status(), model.Calls(), completedCalls, approvalAttempts, restored.Failure())
	}
}

func TestManagedInteractionCancellationAtRequestBoundarySkipsProviderCall(t *testing.T) {
	model := &managedFinalModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	a := agent.New(agent.AgentConfig{Name: "managed-cancel", Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("run")))
		if err != nil {
			return "", err
		}
		_, err = pc.Interact(ctx, core.Interaction{Model: model, Request: request, Tools: registry, Observe: func(_ context.Context, boundary interaction.Event) error {
			if boundary.Kind == interaction.EventModelRequest {
				cancel()
			}
			return nil
		}})
		return "", err
	}, core.ActionConfig{Retry: core.RetryPolicy{MaxAttempts: 3, Safety: core.RetrySafetyIdempotent}})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)

	proc, done := engine.Start(ctx, a, managedInput(), core.ProcessOptions{})
	if err := <-done; err != nil {
		t.Fatalf("run control-flow error = %v", err)
	}
	if proc.Status() != core.StatusKilled || !errors.Is(proc.Failure(), context.Canceled) {
		t.Fatalf("canceled process status=%s failure=%v", proc.Status(), proc.Failure())
	}
	if model.Calls() != 0 {
		t.Fatalf("provider calls = %d, want 0", model.Calls())
	}
}

func managedInteractionAgent(t *testing.T, name string, model chat.Model, registry *tools.Registry, limits interaction.Limits) *core.Agent {
	t.Helper()
	return agent.New(agent.AgentConfig{Name: name, Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request := &chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("run"))}, Tools: registry.Definitions()}
		result, err := pc.Interact(ctx, core.Interaction{Model: model, Request: request, Tools: registry, Limits: limits})
		if err != nil {
			return "", err
		}
		if result.StopReason != interaction.StopNone {
			return string(result.StopReason), nil
		}
		if result.Final == nil || result.Final.Response == nil {
			return "", nil
		}
		return result.Final.Response.Text(), nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
}

func managedInput() core.Bindings {
	return core.Input(struct{}{})
}
