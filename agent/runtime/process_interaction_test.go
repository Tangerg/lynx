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
	"github.com/Tangerg/lynx/tool"
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
	approval := newRuntimeTestTool("approval", func(context.Context) (string, error) {
		return "approved", nil
	})
	registry, err := tools.NewRegistry(approval)
	if err != nil {
		t.Fatal(err)
	}

	var boundaries []event.InteractionBoundary
	listener := runtime.NewEventListener("managed-boundaries", func(_ context.Context, value event.Event) {
		if boundary, ok := value.(event.InteractionBoundary); ok {
			boundaries = append(boundaries, boundary)
		}
	})
	a := managedInteractionAgent(t, "managed-events", registry, interaction.Limits{})
	engine := agent.MustNewEngine(runtime.Config{
		Chat:       core.ChatCapability{Model: model},
		Extensions: []core.Extension{listener},
	})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if usage := proc.Usage(); model.Calls() != 2 || usage.ModelCalls != 2 {
		t.Fatalf("model calls = %d, recorded calls = %d", model.Calls(), usage.ModelCalls)
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
	registered := make([]tool.Tool, 0, len(releases))
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
	a := managedInteractionAgent(t, "managed-concurrency-limit", registry, interaction.Limits{
		MaxConcurrentToolCalls: 2,
	})
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine, a)

	segment, err := engine.Start(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	proc := segment.Process()
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
	if completion := awaitSegment(t, segment); completion.Error() != nil {
		t.Fatal(completion.Error())
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
	a := managedInteractionAgent(t, "managed-negative-concurrency", registry, interaction.Limits{
		MaxConcurrentToolCalls: -1,
	})
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
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
	approval := newRuntimeTestTool("approval", func(ctx context.Context) (string, error) {
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
	registry, err := tools.NewRegistry(approval)
	if err != nil {
		t.Fatal(err)
	}
	a := managedInteractionAgent(t, "managed-resume", registry, interaction.Limits{})
	var boundaries []interaction.EventKind
	listener := runtime.NewEventListener("managed-resume-boundaries", func(_ context.Context, value event.Event) {
		if boundary, ok := value.(event.InteractionBoundary); ok {
			boundaries = append(boundaries, boundary.Boundary.Kind)
		}
	})
	engine := agent.MustNewEngine(runtime.Config{
		Chat:       core.ChatCapability{Model: model},
		Extensions: []core.Extension{listener},
	})
	mustDeploy(t, engine, a)

	segment, err := engine.Start(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if completion := awaitSegment(t, segment); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	proc := segment.Process()
	if proc.Status() != core.StatusWaiting || proc.Suspension() == nil {
		t.Fatalf("parked process = status %s suspension %#v", proc.Status(), proc.Suspension())
	}
	snapshot := snapshotRoot(t, engine, proc)
	if err := runtime.ValidateResumableSnapshot(snapshot); err != nil {
		t.Fatalf("ValidateResumableSnapshot: %v", err)
	}
	// A managed checkpoint that no longer describes its own snapshot is as
	// unrestorable as a malformed one, and a host recovery policy tells all of
	// these apart from an infrastructural failure by one sentinel.
	corrupt := map[string]func(*core.ProcessSnapshot){
		"malformed checkpoint envelope": func(s *core.ProcessSnapshot) {
			s.Suspension.FrameworkState = json.RawMessage(`{}`)
		},
		"checkpoint belongs to another deployment": func(s *core.ProcessSnapshot) {
			s.Deployment = core.DeploymentRef{Name: "someone-else", Digest: s.Deployment.Digest}
		},
		"checkpoint answers another suspension": func(s *core.ProcessSnapshot) {
			s.Suspension.ID = "a-different-pending-call"
		},
	}
	for name, corruption := range corrupt {
		invalid := snapshot
		invalid.Suspension = snapshot.Suspension.Clone()
		corruption(&invalid)
		err := runtime.ValidateResumableSnapshot(invalid)
		if err == nil {
			t.Fatalf("ValidateResumableSnapshot accepted a checkpoint that %s", name)
		}
		if !errors.Is(err, core.ErrInvalidSnapshot) {
			t.Fatalf("%s: error = %v, want it to report ErrInvalidSnapshot", name, err)
		}
	}
	if model.Calls() != 1 || attempts != 1 {
		t.Fatalf("before resume model=%d tool=%d", model.Calls(), attempts)
	}
	resumed, err := engine.ResumeAsync(t.Context(), t.Context(), proc.ID(), "approval-1", true)
	if err != nil {
		t.Fatal(err)
	}
	if completion := awaitSegment(t, resumed); completion.Error() != nil {
		t.Fatal(completion.Error())
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

func TestPendingSuspensionsReportsConcurrentCallsInModelOrder(t *testing.T) {
	model := &managedConcurrentModel{}
	registered := make([]tool.Tool, 0, 2)
	for _, definition := range []struct {
		name         string
		suspensionID string
	}{
		{name: "first", suspensionID: "approval-1"},
		{name: "second", suspensionID: "approval-2"},
	} {
		registered = append(registered, &managedConcurrentTool{
			name: definition.name,
			call: func(ctx context.Context) (string, error) {
				approved, err := hitl.Interrupt[bool](
					ctx,
					definition.suspensionID,
					map[string]any{"tool": definition.name},
				)
				if err != nil {
					return "", err
				}
				if !approved {
					return "denied", nil
				}
				return "approved", nil
			},
		})
	}
	registry, err := tools.NewRegistry(registered...)
	if err != nil {
		t.Fatal(err)
	}
	a := managedInteractionAgent(
		t,
		"managed-pending-suspensions",
		registry,
		interaction.Limits{MaxConcurrentToolCalls: 2},
	)
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine, a)

	segment, err := engine.Start(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if completion := awaitSegment(t, segment); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	process := segment.Process()
	if process.Status() != core.StatusWaiting {
		t.Fatalf("process status = %s, want waiting", process.Status())
	}

	pending, err := engine.PendingSuspensions(t.Context(), process.ID())
	if err != nil {
		t.Fatalf("PendingSuspensions: %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("pending suspensions = %#v, want two", pending)
	}
	for index, wantID := range []string{"approval-1", "approval-2"} {
		if pending[index].ProcessID != process.ID() ||
			pending[index].SuspensionID != wantID {
			t.Fatalf("pending[%d] = %#v, want process %q suspension %q",
				index, pending[index], process.ID(), wantID)
		}
	}
	tree, err := engine.SnapshotTree(t.Context(), process.ID())
	if err != nil {
		t.Fatalf("SnapshotTree: %v", err)
	}
	fromCapture, err := runtime.PendingSuspensionsIn(tree)
	if err != nil {
		t.Fatalf("PendingSuspensionsIn: %v", err)
	}
	if len(fromCapture) != len(pending) {
		t.Fatalf("captured pending suspensions = %#v, want %#v", fromCapture, pending)
	}
	for index := range pending {
		if fromCapture[index].ProcessID != pending[index].ProcessID ||
			fromCapture[index].SuspensionID != pending[index].SuspensionID {
			t.Fatalf("captured pending[%d] = %#v, want %#v", index, fromCapture[index], pending[index])
		}
	}

	// Results are ownership-isolated just like snapshots: protocol bytes can be
	// changed by a caller without corrupting the parked process or a later read.
	pending[0].Prompt[0] = 'x'
	fromCapture[0].Prompt[0] = 'y'
	again, err := engine.PendingSuspensions(t.Context(), process.ID())
	if err != nil {
		t.Fatalf("PendingSuspensions again: %v", err)
	}
	if again[0].Prompt[0] == 'x' {
		t.Fatal("PendingSuspensions returned mutable runtime state")
	}
	if again[0].Prompt[0] == 'y' {
		t.Fatal("PendingSuspensionsIn returned mutable snapshot state")
	}
}

func TestManagedInteractionStopsBeforeContinuationAtStepLimit(t *testing.T) {
	model := &managedModel{}
	approval := newRuntimeTestTool("approval", func(context.Context) (string, error) { return "ok", nil })
	registry, err := tools.NewRegistry(approval)
	if err != nil {
		t.Fatal(err)
	}
	a := managedInteractionAgent(t, "managed-steps", registry, interaction.Limits{MaxRounds: 1})
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if model.Calls() != 1 || proc.Status() != core.StatusCompleted {
		t.Fatalf("model calls=%d status=%s", model.Calls(), proc.Status())
	}
	if result, ok := agent.Result[string](proc); !ok || result != string(interaction.StopSteps) {
		t.Fatalf("result = %q/%v, want %q", result, ok, interaction.StopSteps)
	}
}

func TestManagedInteractionStopsBeforeContinuationAtModelCallLimit(t *testing.T) {
	model := &managedModel{}
	approval := newRuntimeTestTool("approval", func(context.Context) (string, error) { return "ok", nil })
	registry, err := tools.NewRegistry(approval)
	if err != nil {
		t.Fatal(err)
	}
	a := managedInteractionAgent(t, "managed-model-calls", registry, interaction.Limits{})
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{
		Budget: core.Budget{ModelCallLimit: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if usage := proc.Usage(); model.Calls() != 1 || usage.ModelCalls != 1 || proc.Status() != core.StatusCompleted {
		t.Fatalf("provider calls=%d recorded calls=%d status=%s", model.Calls(), usage.ModelCalls, proc.Status())
	}
}

// managedRejectedResponseModel answers its first call with a response the
// framework rejects — a negative token count — and its second normally.
// Providers assign Usage after the response is built, so the rejection lands
// inside the managed interaction, after the call was already admitted.
type managedRejectedResponseModel struct {
	mu    sync.Mutex
	calls int
}

func (m *managedRejectedResponseModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	message := chat.NewAssistantMessage(chat.NewTextPart("complete"))
	response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	if err != nil {
		return nil, err
	}
	response.Model = "managed-rejected-response"
	if m.calls == 1 {
		response.Usage = chat.Usage{InputTokens: -1}
		return response, nil
	}
	response.Usage = chat.Usage{InputTokens: 4, OutputTokens: 1}
	return response, nil
}

func (m *managedRejectedResponseModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func TestManagedInteractionKeepsModelCallCapacityWhenResponseIsRejected(t *testing.T) {
	model := &managedRejectedResponseModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var firstErr error
	a := agent.New(agent.AgentConfig{Name: "managed-rejected-response", Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("run")))
		if err != nil {
			return "", err
		}
		// The first interaction fails after its model call was admitted but
		// before that call was charged. An action may legitimately continue, so
		// the reserved capacity has to be back for the retry.
		_, firstErr = pc.Interact(ctx, core.Interaction{Request: request, Tools: registry})
		result, err := pc.Interact(ctx, core.Interaction{Request: request, Tools: registry})
		if err != nil {
			return "", err
		}
		if result.StopReason != interaction.StopNone {
			return string(result.StopReason), nil
		}
		return result.Final.Response.Text(), nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{
		Budget: core.Budget{ModelCallLimit: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if firstErr == nil {
		t.Fatal("first interaction reported no error for an unrecordable token count")
	}
	result, ok := agent.Result[string](proc)
	if !ok || result != "complete" {
		t.Fatalf("result = %q (present=%t), want the retry to complete", result, ok)
	}
	if model.Calls() != 2 || proc.Usage().ModelCalls != 1 {
		t.Fatalf("provider calls=%d recorded calls=%d, want the failed call to leave no reserved capacity",
			model.Calls(), proc.Usage().ModelCalls)
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
	message := chat.NewAssistantMessage(chat.NewTextPart("complete"))
	response, err := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	response.Model = "managed-final"
	return response, err
}

func (m *managedFinalModel) Calls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type managedInteractionExtension struct {
	name    string
	cost    func(*chat.Response) (float64, error)
	observe func(interaction.Event)
}

func (e managedInteractionExtension) Name() string { return e.name }

func (e managedInteractionExtension) ProjectInteractionCost(
	_ context.Context,
	_ core.ProcessView,
	response *chat.Response,
) (float64, error) {
	if e.cost == nil {
		return 0, nil
	}
	return e.cost(response)
}

func (e managedInteractionExtension) OnEvent(_ context.Context, published event.Event) {
	if e.observe == nil {
		return
	}
	boundary, ok := published.(event.InteractionBoundary)
	if ok {
		e.observe(boundary.Boundary)
	}
}

func TestManagedInteractionListenerPanicDoesNotFailModelResponse(t *testing.T) {
	model := &managedFinalModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	a := agent.New(agent.AgentConfig{Name: "managed-listener-panic", Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("run")))
		if err != nil {
			return "", err
		}
		result, err := pc.Interact(ctx, core.Interaction{Request: request, Tools: registry})
		if err != nil {
			return "", err
		}
		return result.Final.Response.Text(), nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	engine := agent.MustNewEngine(runtime.Config{
		Chat: core.ChatCapability{Model: model},
		Extensions: []core.Extension{managedInteractionExtension{
			name: "listener-panic",
			observe: func(boundary interaction.Event) {
				if boundary.Kind == interaction.EventModelResponse {
					panic("listener stopped")
				}
			},
		}},
	})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run control-flow error = %v", err)
	}
	if proc == nil {
		t.Fatal("Run returned nil process")
	}
	if proc.Status() != core.StatusCompleted || proc.Failure() != nil {
		t.Fatalf("process status=%v failure=%v", proc.Status(), proc.Failure())
	}
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want one action execution", model.Calls())
	}
}

func TestManagedInteractionRecordsHostCostAndPublishesIt(t *testing.T) {
	model := &managedFinalModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var observedCost float64
	a := agent.New(agent.AgentConfig{Name: "managed-cost", Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("run")))
		if err != nil {
			return "", err
		}
		result, err := pc.Interact(ctx, core.Interaction{
			Request: request,
			Tools:   registry,
		})
		if err != nil {
			return "", err
		}
		return result.Final.Response.Text(), nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	engine := agent.MustNewEngine(runtime.Config{
		Chat: core.ChatCapability{Model: model},
		Extensions: []core.Extension{managedInteractionExtension{
			name: "cost-projection",
			cost: func(*chat.Response) (float64, error) { return 0.25, nil },
			observe: func(boundary interaction.Event) {
				if boundary.Kind == interaction.EventModelResponse {
					observedCost = boundary.Cost
				}
			},
		}},
	})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if usage := proc.Usage(); usage.Cost != 0.25 || usage.ModelCalls != 1 || observedCost != 0.25 {
		t.Fatalf("usage=%+v observed cost=%v, want one call costing 0.25", usage, observedCost)
	}
}

func TestManagedInteractionIsolatesProjectorsAndObservers(t *testing.T) {
	model := &managedFinalModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var observedModel string
	a := agent.New(agent.AgentConfig{Name: "managed-isolated-projections", Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("run")))
		if err != nil {
			return "", err
		}
		result, err := pc.Interact(ctx, core.Interaction{Request: request, Tools: registry})
		if err != nil {
			return "", err
		}
		return result.Final.Response.Model, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	engine := agent.MustNewEngine(runtime.Config{
		Chat: core.ChatCapability{Model: model},
		Extensions: []core.Extension{
			managedInteractionExtension{
				name: "mutating-projection",
				cost: func(response *chat.Response) (float64, error) {
					response.Model = "projector-mutated"
					return 0.25, nil
				},
				observe: func(boundary interaction.Event) {
					if boundary.Kind == interaction.EventModelResponse {
						boundary.Response.Model = "observer-mutated"
					}
				},
			},
			managedInteractionExtension{
				name: "recording-observer",
				observe: func(boundary interaction.Event) {
					if boundary.Kind == interaction.EventModelResponse {
						observedModel = boundary.Response.Model
					}
				},
			},
		},
	})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	result, ok := agent.Result[string](proc)
	if !ok || result != "managed-final" || observedModel != "managed-final" {
		t.Fatalf("result=%q/%v observed=%q, want isolated managed-final values", result, ok, observedModel)
	}
	if usage := proc.Usage(); usage.Cost != 0.25 {
		t.Fatalf("usage cost=%v, want 0.25", usage.Cost)
	}
}

func TestManagedInteractionContainsCostProjectorPanic(t *testing.T) {
	model := &managedFinalModel{}
	registry, err := tools.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("pricing failed")
	var observedResponse bool
	a := agent.New(agent.AgentConfig{Name: "managed-cost-panic", Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("run")))
		if err != nil {
			return "", err
		}
		_, err = pc.Interact(ctx, core.Interaction{Request: request, Tools: registry})
		return "", err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	engine := agent.MustNewEngine(runtime.Config{
		Chat: core.ChatCapability{Model: model},
		Extensions: []core.Extension{managedInteractionExtension{
			name: "cost-panic",
			cost: func(*chat.Response) (float64, error) {
				panic(cause)
			},
			observe: func(boundary interaction.Event) {
				if boundary.Kind == interaction.EventModelResponse {
					observedResponse = true
				}
			},
		}},
	})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil || proc == nil || proc.Status() != core.StatusFailed || !errors.Is(proc.Failure(), cause) {
		t.Fatalf("process=%v error=%v, want failed process wrapping cause", proc, err)
	}
	if usage := proc.Usage(); usage.ModelCalls != 1 || usage.Tokens != 0 || usage.Cost != 0 {
		t.Fatalf("resource usage after rejected cost = %+v, want one known model call at unknown cost", usage)
	}
	if !observedResponse {
		t.Fatal("executed model response was hidden after cost projection failed")
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
	first := newRuntimeTestTool("first", func(context.Context) (string, error) {
		completedCalls++
		return "first done", nil
	})
	approval := newRuntimeTestTool("approval", func(ctx context.Context) (string, error) {
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
	registry, err := tools.NewRegistry(first, approval)
	if err != nil {
		t.Fatal(err)
	}
	a := managedInteractionAgent(t, "managed-crash-restore", registry, interaction.Limits{})
	engine1 := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine1, a)

	segment, err := engine1.Start(t.Context(), a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if completion := awaitSegment(t, segment); completion.Error() != nil {
		t.Fatal(completion.Error())
	}
	proc := segment.Process()
	if proc.Status() != core.StatusWaiting || model.Calls() != 1 || completedCalls != 1 || approvalAttempts != 1 {
		t.Fatalf("before crash status=%s model=%d completed=%d approval=%d", proc.Status(), model.Calls(), completedCalls, approvalAttempts)
	}

	captured := snapshotRoot(t, engine1, proc)
	body, err := json.Marshal(captured)
	if err != nil {
		t.Fatalf("marshal crash snapshot: %v", err)
	}
	var snapshot core.ProcessSnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		t.Fatalf("unmarshal crash snapshot: %v", err)
	}
	engine2 := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine2, a)
	restored := restoreRoot(t, engine2, snapshot, core.ProcessOptions{})
	if err := engine2.Resume(t.Context(), restored.ID(), "approval-crash", true); err != nil {
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
		_, err = pc.Interact(ctx, core.Interaction{Request: request, Tools: registry})
		return "", err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[string](core.GoalConfig{Description: "managed result"})}})
	cancelListener := runtime.NewEventListener("cancel-at-request", func(_ context.Context, value event.Event) {
		if boundary, ok := value.(event.InteractionBoundary); ok &&
			boundary.Boundary.Kind == interaction.EventModelRequest {
			cancel()
		}
	})
	engine := agent.MustNewEngine(runtime.Config{
		Chat:       core.ChatCapability{Model: model},
		Extensions: []core.Extension{cancelListener},
	})
	mustDeploy(t, engine, a)

	segment, err := engine.Start(ctx, a, managedInput(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	completion := awaitSegment(t, segment)
	if completion.Err != nil {
		t.Fatalf("run control-flow error = %v", completion.Err)
	}
	proc := segment.Process()
	if proc.Status() != core.StatusKilled || !errors.Is(proc.Failure(), context.Canceled) {
		t.Fatalf("canceled process status=%s failure=%v", proc.Status(), proc.Failure())
	}
	if model.Calls() != 0 {
		t.Fatalf("provider calls = %d, want 0", model.Calls())
	}
}

func managedInteractionAgent(t *testing.T, name string, registry *tools.Registry, limits interaction.Limits) *core.Agent {
	t.Helper()
	return agent.New(agent.AgentConfig{Name: name, Actions: []agent.Action{agent.NewAction("interact", func(ctx context.Context, pc *core.ProcessContext, _ struct{}) (string, error) {
		request := &chat.Request{Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("run"))}, Tools: registry.Definitions()}
		result, err := pc.Interact(ctx, core.Interaction{Request: request, Tools: registry, Limits: limits})
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

func TestManagedInteractionInheritsProcessToolRoundLimit(t *testing.T) {
	model := &managedModel{}
	approval := newRuntimeTestTool("approval", func(context.Context) (string, error) {
		return "approved", nil
	})
	registry, err := tools.NewRegistry(approval)
	if err != nil {
		t.Fatal(err)
	}
	// The interaction states no limit of its own, so the process limit governs
	// it — the framework never substitutes a round count of its own. Reaching a
	// limit the host configured is an outcome, so the interaction reports
	// StopSteps and the process completes instead of failing.
	a := managedInteractionAgent(t, "managed-process-rounds", registry, interaction.Limits{})
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	mustDeploy(t, engine, a)

	proc, err := engine.Run(t.Context(), a, managedInput(), core.ProcessOptions{MaxToolRounds: 1})
	if err != nil {
		t.Fatal(err)
	}
	if proc.Status() != core.StatusCompleted || proc.Failure() != nil {
		t.Fatalf("status = %s, failure = %v; want completed without failure", proc.Status(), proc.Failure())
	}
	if result, ok := agent.Result[string](proc); !ok || result != string(interaction.StopSteps) {
		t.Fatalf("result = %q/%v, want %q", result, ok, interaction.StopSteps)
	}
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want the process limit to stop the loop after one round", model.Calls())
	}
}
