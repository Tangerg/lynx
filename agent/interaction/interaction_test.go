package interaction_test

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/interaction"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tool"
)

func TestManagedInteractionCompletesFromModelResponse(t *testing.T) {
	model := chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		if len(request.Tools) != 0 {
			t.Fatalf("model request has %d tools, want none", len(request.Tools))
		}
		return textResponse("done"), nil
	})
	deployment := newDeployment(t, model, nil, 2)
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	input, err := agent.EncodeInput(interaction.Input{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("finish"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s, want completed; termination = %#v", result.Status(), result.Termination())
	}
	erased, ok := result.Output()
	if !ok {
		t.Fatal("completed Interaction has no output")
	}
	output, err := erased.Decode[interaction.Output]()
	if err != nil {
		t.Fatal(err)
	}
	if output.Source != interaction.CompletionSourceModelResponse || output.ModelResponse == nil ||
		output.ModelCalls != 1 || output.ModelResponse.Text() != "done" {
		t.Fatalf("output = %#v", output)
	}
}

func TestManagedInteractionExecutesToolLoopInModelOrder(t *testing.T) {
	type addInput struct {
		Left  int `json:"left"`
		Right int `json:"right"`
	}
	add, err := tool.NewFunc(tool.FuncConfig{
		Name:        "add",
		Description: "Add two integers and return their sum.",
	}, func(_ context.Context, input addInput) (int, error) {
		return input.Left + input.Right, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	model := &scriptedModel{}
	deployment := newDeployment(t, model, []tool.Tool{add}, 3)
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := engine.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	input, err := agent.EncodeInput(interaction.Input{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("add 2 and 3"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), deployment, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted {
		t.Fatalf("status = %s, termination = %#v", result.Status(), result.Termination())
	}
	erased, _ := result.Output()
	output, err := erased.Decode[interaction.Output]()
	if err != nil {
		t.Fatal(err)
	}
	if output.Source != interaction.CompletionSourceModelResponse || output.ModelResponse == nil ||
		output.ModelCalls != 2 || output.ModelResponse.Text() != "5" {
		t.Fatalf("output = %#v", output)
	}
	if model.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2", model.Calls())
	}
}

func TestManagedInteractionTerminatesOnModelHostFailure(t *testing.T) {
	model := chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, interaction.HostFailure(errors.New("model boundary unavailable"))
	})
	result := runInteraction(t, newDeployment(t, model, nil, 2), "fail before provider")
	assertInteractionHostFailure(t, result)
}

func TestManagedInteractionTerminatesOnToolHostFailureWithoutAnotherModelCall(t *testing.T) {
	type input struct{}
	failing, err := tool.NewFunc(tool.FuncConfig{
		Name: "failing", Description: "Fail at the host boundary.",
	}, func(context.Context, input) (string, error) {
		return "", interaction.HostFailure(errors.New("tool boundary unavailable"))
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &singleToolCallModel{call: chat.ToolCall{ID: "call_host", Name: "failing", Arguments: `{}`}}
	result := runInteraction(t, newDeployment(t, model, []tool.Tool{failing}, 2), "fail before Tool")
	assertInteractionHostFailure(t, result)
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want no retry after host failure", model.Calls())
	}
}

func runInteraction(t *testing.T, deployment agent.Deployment, prompt string) agent.Result {
	t.Helper()
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), deployment, interactionInput(t, prompt))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	return result
}

func assertInteractionHostFailure(t *testing.T, result agent.Result) {
	t.Helper()
	failure, ok := result.Termination().Failure()
	if result.Status() != agent.StatusFailed || !ok ||
		failure.Kind() != agent.FailureKindExternal || failure.Code() != "interaction.host.failed" {
		t.Fatalf("result = status:%s termination:%#v", result.Status(), result.Termination())
	}
}

func TestDefinitionRejectsZeroModelCallLimit(t *testing.T) {
	_, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name: "interaction.test", Description: "Run a test interaction.", Version: "1.0.0",
	})
	if !errors.Is(err, interaction.ErrInvalidDefinitionConfig) {
		t.Fatalf("error = %v, want ErrInvalidDefinitionConfig", err)
	}
}

func TestDefinitionRestoresCompleteWorkingContext(t *testing.T) {
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:          "interaction.restore",
		Description:   "Verify exact Interaction state restoration.",
		Version:       "1.0.0",
		MaxModelCalls: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := agent.EncodeInput(interaction.Input{
		Messages: []chat.Message{chat.NewUserMessage(chat.NewTextPart("persist me"))},
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := definition.Start(input)
	if err != nil {
		t.Fatal(err)
	}
	transition, err := execution.Step(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if transition.Kind() != agent.TransitionKindContinue || len(transition.Effects()) != 1 {
		t.Fatalf("transition = %#v", transition)
	}
	before, err := execution.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored, err := definition.Restore(before)
	if err != nil {
		t.Fatal(err)
	}
	after, err := restored.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.Payload(), after.Payload()) {
		t.Fatalf("restored payload differs\nbefore: %s\nafter:  %s", before.Payload(), after.Payload())
	}
}

func TestDirectResultToolCompletesWithoutAnotherModelCall(t *testing.T) {
	type input struct {
		Value string `json:"value"`
	}
	echo, err := tool.NewFunc(tool.FuncConfig{
		Name:        "echo",
		Description: "Return the supplied value directly.",
	}, func(_ context.Context, input input) (string, error) {
		return input.Value, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	model := &singleToolCallModel{call: chat.ToolCall{ID: "call_direct", Name: "echo", Arguments: `{"value":"direct"}`}}
	deployment := newDeployment(t, model, []tool.Tool{directTool{Tool: echo}}, 2)
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), deployment, interactionInput(t, "direct"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusCompleted || model.Calls() != 1 {
		t.Fatalf("status = %s, model calls = %d", result.Status(), model.Calls())
	}
	erased, _ := result.Output()
	output, err := erased.Decode[interaction.Output]()
	if err != nil {
		t.Fatal(err)
	}
	if output.Source != interaction.CompletionSourceDirectToolResults || output.ModelResponse != nil ||
		len(output.DirectToolResults) != 1 || output.DirectToolResults[0].Result != "direct" {
		t.Fatalf("output = %#v", output)
	}
}

func TestModelCallLimitProducesStableFailure(t *testing.T) {
	type noInput struct{}
	next, err := tool.NewFunc(tool.FuncConfig{
		Name:        "next",
		Description: "Request another model round.",
	}, func(context.Context, noInput) (string, error) { return "continue", nil })
	if err != nil {
		t.Fatal(err)
	}
	model := &singleToolCallModel{call: chat.ToolCall{ID: "call_limit", Name: "next", Arguments: `{}`}}
	deployment := newDeployment(t, model, []tool.Tool{next}, 1)
	engine, err := agent.NewEngine(agent.EngineConfig{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), deployment, interactionInput(t, "loop"))
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	if result.Status() != agent.StatusFailed || model.Calls() != 1 {
		t.Fatalf("status = %s, model calls = %d", result.Status(), model.Calls())
	}
	failure, ok := result.Termination().Failure()
	if !ok || failure.Code() != "interaction.limit.model_calls" || failure.Kind() != agent.FailureKindExecution {
		t.Fatalf("failure = %#v, present = %t", failure, ok)
	}
}

type scriptedModel struct {
	mu    sync.Mutex
	calls int
}

type singleToolCallModel struct {
	mu    sync.Mutex
	calls int
	call  chat.ToolCall
}

func (model *singleToolCallModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	return toolCallResponse(model.call), nil
}

func (model *singleToolCallModel) Calls() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return model.calls
}

type directTool struct {
	tool.Tool
}

func (value directTool) Unwrap() tool.Tool { return value.Tool }

func (directTool) ReturnsDirectResult() bool { return true }

func (model *scriptedModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	model.mu.Lock()
	defer model.mu.Unlock()
	model.calls++
	if len(request.Tools) != 1 || request.Tools[0].Name != "add" {
		return nil, errors.New("tool manifest is not frozen into model request")
	}
	if model.calls == 1 {
		if len(request.Messages) != 1 {
			return nil, errors.New("first request has unexpected WorkingContext")
		}
		return toolCallResponse(chat.ToolCall{ID: "call_1", Name: "add", Arguments: `{"left":2,"right":3}`}), nil
	}
	if len(request.Messages) != 3 || request.Messages[1].Role != chat.RoleAssistant || request.Messages[2].Role != chat.RoleTool {
		return nil, errors.New("tool continuation does not preserve model order")
	}
	result := request.Messages[2].Parts[0].ToolResult
	if result == nil || result.ID != "call_1" || result.Name != "add" || result.Result != "5" {
		return nil, errors.New("tool continuation contains the wrong result")
	}
	return textResponse("5"), nil
}

func (model *scriptedModel) Calls() int {
	model.mu.Lock()
	defer model.mu.Unlock()
	return model.calls
}

func newDeployment(t *testing.T, model chat.Model, tools []tool.Tool, maxModelCalls uint32) agent.Deployment {
	t.Helper()
	client, err := chatclient.New(model, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := interaction.NewDefinition(interaction.DefinitionConfig{
		Name:          "interaction.test",
		Description:   "Run a model-directed interaction for contract testing.",
		Version:       "1.0.0",
		MaxModelCalls: maxModelCalls,
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
		ImplementationDigest: agent.ComputeDigest([]byte("interaction-test-implementation")),
		ConfigurationDigest:  agent.ComputeDigest([]byte("interaction-test-configuration")),
	})
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

func textResponse(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	return &chat.Response{Result: &chat.Result{
		Message:      &message,
		FinishReason: chat.FinishReasonStop,
	}}
}

func toolCallResponse(call chat.ToolCall) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
	return &chat.Response{Result: &chat.Result{
		Message:      &message,
		FinishReason: chat.FinishReasonToolCalls,
	}}
}
