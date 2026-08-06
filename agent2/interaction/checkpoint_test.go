package interaction_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agent "github.com/Tangerg/lynx/agent2"
	"github.com/Tangerg/lynx/agent2/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

func TestToolCheckpointRestoresWithoutRepeatingSettledPrefix(t *testing.T) {
	var prefixCalls atomic.Int32
	prefix, err := tool.NewFunc(tool.FuncConfig{
		Name:        "prefix",
		Description: "Complete before the next tool requests input.",
	}, func(context.Context, struct{}) (string, error) {
		prefixCalls.Add(1)
		return "prefix-complete", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	waiting := newInputRequestTool()
	t.Cleanup(waiting.Release)
	model := &checkpointModel{}
	deployment := checkpointDeployment(t, model, []tool.Tool{prefix, waiting})

	firstEngine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	process, err := firstEngine.Start(context.Background(), deployment, interactionInput(t, "checkpoint"))
	if err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, process, agent.StatusWaiting)
	waitID, ok := process.WaitID()
	if !ok {
		t.Fatal("waiting Process has no WaitID")
	}
	livePending, found, err := interaction.PendingToolInputFromProcess(context.Background(), process)
	if err != nil || !found || livePending.WaitID() != waitID {
		t.Fatalf("PendingToolInputFromProcess found = %t, WaitID = %s, error = %v", found, livePending.WaitID().String(), err)
	}
	snapshot, err := process.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := process.Kill(context.Background(), "replace with restored Process"); err != nil {
		t.Fatal(err)
	}
	if _, err := process.Await(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := firstEngine.Close(); err != nil {
		t.Fatal(err)
	}
	pending, found, err := interaction.PendingToolInputFromSnapshot(snapshot)
	if err != nil || !found {
		t.Fatalf("PendingToolInputFromSnapshot found = %t, error = %v", found, err)
	}
	if pending.WaitID() != waitID || string(pending.Prompt()) != `{"question":"What is your name?"}` {
		t.Fatalf("pending input = WaitID %s, prompt %s", pending.WaitID().String(), pending.Prompt())
	}

	restoredEngine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	signalID, err := agent.ParseSignalID("signal:checkpoint-answer")
	if err != nil {
		t.Fatal(err)
	}
	invalidID, err := agent.ParseSignalID("signal:invalid-checkpoint-answer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pending.ResponseSignal(invalidID, json.RawMessage(`42`)); !errors.Is(err, interaction.ErrInvalidToolInputRequest) {
		t.Fatalf("invalid response error = %v, want ErrInvalidToolInputRequest", err)
	}
	wrongWaitID, err := agent.ParseWaitID("wait:wrong")
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := interaction.NewToolInputResponseSignal(invalidID, wrongWaitID, json.RawMessage(`"Ada"`))
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err := restored.DeliverSignal(context.Background(), wrong); accepted || !errors.Is(err, agent.ErrSignalRejected) {
		t.Fatalf("wrong-wait delivery accepted = %t, error = %v", accepted, err)
	}
	answer, err := pending.ResponseSignal(signalID, json.RawMessage(`"Ada"`))
	if err != nil {
		t.Fatal(err)
	}
	accepted, err := restored.DeliverSignal(context.Background(), answer)
	if err != nil || !accepted {
		t.Fatalf("DeliverSignal accepted = %t, error = %v", accepted, err)
	}
	<-waiting.continuationStarted
	accepted, err = restored.DeliverSignal(context.Background(), answer)
	if err != nil || accepted {
		t.Fatalf("duplicate DeliverSignal accepted = %t, error = %v", accepted, err)
	}
	waiting.Release()
	result, err := restored.Await(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s, termination = %#v", result.Status(), result.Termination())
	}
	if prefixCalls.Load() != 1 {
		t.Fatalf("settled prefix calls = %d, want 1", prefixCalls.Load())
	}
	if waiting.initialCalls.Load() != 1 || waiting.continuationCalls.Load() != 1 {
		t.Fatalf("input tool initial/continuation calls = %d/%d, want 1/1", waiting.initialCalls.Load(), waiting.continuationCalls.Load())
	}
	if model.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2", model.Calls())
	}
	erased, _ := result.Output()
	output, err := agent.DecodeOutput[interaction.Output](erased)
	if err != nil {
		t.Fatal(err)
	}
	if output.ModelResponse == nil || output.ModelResponse.Text() != "prefix-complete; hello Ada" {
		t.Fatalf("output = %#v", output)
	}
	staleID, err := agent.ParseSignalID("signal:stale-checkpoint-answer")
	if err != nil {
		t.Fatal(err)
	}
	stale, err := pending.ResponseSignal(staleID, json.RawMessage(`"Grace"`))
	if err != nil {
		t.Fatal(err)
	}
	if accepted, err := restored.DeliverSignal(context.Background(), stale); accepted || !errors.Is(err, agent.ErrProcessFinished) {
		t.Fatalf("stale delivery accepted = %t, error = %v", accepted, err)
	}
}

func TestPreparedToolEffectIsNotRetriedAfterRestore(t *testing.T) {
	blocking := newBlockingTool()
	t.Cleanup(blocking.Release)
	model := &singleToolCallModel{call: chat.ToolCall{ID: "call_block", Name: "block", Arguments: `{}`}}
	deployment := checkpointDeployment(t, model, []tool.Tool{blocking})
	firstEngine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	process, err := firstEngine.Start(context.Background(), deployment, interactionInput(t, "block"))
	if err != nil {
		t.Fatal(err)
	}
	<-blocking.started
	snapshot, err := process.Capture(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	restoredEngine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := restoredEngine.Restore(context.Background(), deployment, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := restored.UnknownEffectIDs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(unknown) != 1 {
		t.Fatalf("unknown EffectIDs = %v, want one", unknown)
	}
	if blocking.calls.Load() != 1 {
		t.Fatalf("tool calls after restore = %d, want 1", blocking.calls.Load())
	}
	if err := restored.Kill(context.Background(), "unknown side effect requires caller decision"); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.Await(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := restoredEngine.Close(); err != nil {
		t.Fatal(err)
	}

	if err := process.Kill(context.Background(), "finish original blocked Process"); err != nil {
		t.Fatal(err)
	}
	blocking.Release()
	if _, err := process.Await(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := firstEngine.Close(); err != nil {
		t.Fatal(err)
	}
	if blocking.calls.Load() != 1 {
		t.Fatalf("final tool calls = %d, want 1", blocking.calls.Load())
	}
}

type inputRequestTool struct {
	initialCalls        atomic.Int32
	continuationCalls   atomic.Int32
	continuationStarted chan struct{}
	continuationRelease chan struct{}
	startedOnce         sync.Once
	releaseOnce         sync.Once
}

func newInputRequestTool() *inputRequestTool {
	return &inputRequestTool{
		continuationStarted: make(chan struct{}),
		continuationRelease: make(chan struct{}),
	}
}

func (*inputRequestTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "ask_name",
		Description: "Ask for a name and continue with the validated answer.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (value *inputRequestTool) Call(ctx context.Context, _ string) (string, error) {
	continuation, resumed := interaction.ToolInputContinuationFromContext(ctx)
	if !resumed {
		value.initialCalls.Add(1)
		return "", interaction.RequireToolInput(
			json.RawMessage(`{"question":"What is your name?"}`),
			json.RawMessage(`{"type":"string","minLength":1}`),
			json.RawMessage(`{"stage":"awaiting_name"}`),
		)
	}
	value.continuationCalls.Add(1)
	value.startedOnce.Do(func() { close(value.continuationStarted) })
	<-value.continuationRelease
	var state struct {
		Stage string `json:"stage"`
	}
	if err := json.Unmarshal(continuation.State(), &state); err != nil || state.Stage != "awaiting_name" {
		return "", errors.New("invalid continuation state")
	}
	var name string
	if err := json.Unmarshal(continuation.Response(), &name); err != nil {
		return "", err
	}
	return "hello " + name, nil
}

func (value *inputRequestTool) Release() {
	value.releaseOnce.Do(func() { close(value.continuationRelease) })
}

type checkpointModel struct {
	mu    sync.Mutex
	calls int
}

func (model *checkpointModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	if model.calls == 1 {
		return toolCallsResponse(
			chat.ToolCall{ID: "call_prefix", Name: "prefix", Arguments: `{}`},
			chat.ToolCall{ID: "call_name", Name: "ask_name", Arguments: `{}`},
		), nil
	}
	if len(request.Messages) != 3 || request.Messages[2].Role != chat.RoleTool || len(request.Messages[2].Parts) != 2 {
		return nil, errors.New("restored continuation lost ordered ToolResults")
	}
	first := request.Messages[2].Parts[0].ToolResult
	second := request.Messages[2].Parts[1].ToolResult
	if first == nil || second == nil || first.Result != "prefix-complete" || second.Result != "hello Ada" {
		return nil, errors.New("restored continuation contains incorrect ToolResults")
	}
	return textResponse("prefix-complete; hello Ada"), nil
}

func (model *checkpointModel) Calls() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return model.calls
}

type blockingTool struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
	calls       atomic.Int32
}

func newBlockingTool() *blockingTool {
	return &blockingTool{started: make(chan struct{}), release: make(chan struct{})}
}

func (*blockingTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        "block",
		Description: "Block until the test releases the external operation.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}
}

func (value *blockingTool) Call(context.Context, string) (string, error) {
	value.calls.Add(1)
	value.startedOnce.Do(func() { close(value.started) })
	<-value.release
	return "released", nil
}

func (value *blockingTool) Release() {
	value.releaseOnce.Do(func() { close(value.release) })
}

func checkpointDeployment(t *testing.T, model chat.Model, tools []tool.Tool) agent.Deployment {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:          "interaction.checkpoint",
		Description:   "Verify exact Tool checkpoint and restoration behavior.",
		Version:       "1.0.0",
		MaxModelCalls: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	dispatcher, err := interaction.NewDispatcher(definition, interaction.DispatcherConfig{Client: client, Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := agent.NewDeployment(agent.DeploymentConfig{
		Definition:           definition,
		Dispatcher:           dispatcher,
		ImplementationDigest: agent.ComputeDigest([]byte("interaction-checkpoint-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("interaction-checkpoint-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

func waitForStatus(t *testing.T, process *agent.Process, want agent.Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if process.Status() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("status = %s, want %s", process.Status(), want)
}

func toolCallsResponse(calls ...chat.ToolCall) *chat.Response {
	parts := make([]chat.Part, len(calls))
	for index := range calls {
		parts[index] = chat.NewToolCallPart(calls[index])
	}
	message := chat.NewAssistantMessage(parts...)
	return &chat.Response{Choices: []chat.Choice{{
		Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls,
	}}}
}
