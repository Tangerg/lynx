package workflow_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type toolCallingModel struct{}

func (toolCallingModel) Call(_ context.Context, request *chat.Request) (*chat.Response, error) {
	for index := range request.Messages {
		if request.Messages[index].Role == chat.RoleTool {
			return fakeTextResponse("orchestrated result"), nil
		}
	}
	toolName := request.Tools[0].Name
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
		ID: "call-worker", Name: toolName, Arguments: `{"Title":"go generics"}`,
	}))
	return chat.NewResponse(chat.Choice{
		Index: 0, Message: &message, FinishReason: chat.FinishReasonToolCalls,
	})
}
func fakeTextResponse(text string) *chat.Response {
	message := chat.NewAssistantMessage(chat.NewTextPart(text))
	resp, _ := chat.NewResponse(chat.Choice{Index: 0, Message: &message, FinishReason: chat.FinishReasonStop})
	return resp
}

type supTopic struct{ Title string }
type supAnswer struct{ Text string }

var errRenderInput = errors.New("render input failed")

type unrenderableInput struct{}

func (unrenderableInput) MarshalJSON() ([]byte, error) { return nil, errRenderInput }

type renderGuardModel struct{ calls int }

func (m *renderGuardModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	m.calls++
	return nil, errors.New("model must not be called")
}

func makeSubAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "worker", Actions: []agent.Action{agent.NewAction("work", func(_ context.Context, _ *core.ProcessContext, in supTopic) (supAnswer, error) {
		return supAnswer{Text: "did " + in.Title}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[supAnswer](core.GoalConfig{Name: "worker-goal"})}})
}

func deploySubAgent(t *testing.T, engine *runtime.Engine) *runtime.Deployment {
	t.Helper()
	deployment, err := engine.Deploy(t.Context(), makeSubAgent())
	if err != nil {
		t.Fatal(err)
	}
	return deployment
}

func TestSupervisor_Validation(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	worker, err := runtime.NewAgentTool[supTopic, supAnswer](engine, deploySubAgent(t, engine))
	if err != nil {
		t.Fatal(err)
	}

	parse := func(s string) (supAnswer, error) { return supAnswer{Text: s}, nil }

	cases := []struct {
		name   string
		config workflow.SupervisorConfig[supTopic, supAnswer]
	}{
		{"empty name", workflow.SupervisorConfig[supTopic, supAnswer]{Tools: []tools.Tool{worker}, Parse: parse}},
		{"no tools", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Parse: parse}},
		{"nil tool", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Tools: []tools.Tool{nil}, Parse: parse}},
		{"nil parse", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Tools: []tools.Tool{worker}}},
	}
	for _, test := range cases {
		if _, err := workflow.Supervisor(test.config); err == nil {
			t.Errorf("%s: expected error, got nil", test.name)
		}
	}
}

// TestSupervisor_EndToEnd drives the supervisor with a fake model that
// returns a final answer directly, confirming the chat client wiring and
// Parse path produce the typed output.
func TestSupervisor_EndToEnd(t *testing.T) {
	client, err := chatclient.New(toolCallingModel{})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: client, Streamer: client}})
	worker, err := runtime.NewAgentTool[supTopic, supAnswer](engine, deploySubAgent(t, engine))
	if err != nil {
		t.Fatal(err)
	}

	supervisor, err := workflow.Supervisor(workflow.SupervisorConfig[supTopic, supAnswer]{
		Name:         "supervisor",
		Description:  "orchestrate the worker",
		Tools:        []tools.Tool{worker},
		Instructions: "Use the worker tool, then reply.",
		Parse:        func(text string) (supAnswer, error) { return supAnswer{Text: text}, nil },
	})
	if err != nil {
		t.Fatalf("Supervisor: %v", err)
	}
	mustDeploy(t, engine, supervisor)

	process, err := engine.Run(context.Background(), supervisor,
		core.Input(supTopic{Title: "go generics"}),
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure=%v", process.Status(), process.Failure())
	}

	out, ok := core.Result[supAnswer](process)
	if !ok {
		t.Fatal("no supAnswer produced")
	}
	if out.Text != "orchestrated result" {
		t.Fatalf("output = %q, want %q", out.Text, "orchestrated result")
	}
}

func TestSupervisor_DefaultRenderPropagatesJSONError(t *testing.T) {
	model := new(renderGuardModel)
	engine := agent.MustNewEngine(runtime.Config{Chat: core.ChatCapability{Model: model}})
	worker, err := runtime.NewAgentTool[supTopic, supAnswer](engine, deploySubAgent(t, engine))
	if err != nil {
		t.Fatal(err)
	}
	supervisor, err := workflow.Supervisor(workflow.SupervisorConfig[unrenderableInput, supAnswer]{
		Name:  "render-error",
		Tools: []tools.Tool{worker},
		Parse: func(text string) (supAnswer, error) { return supAnswer{Text: text}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	process, err := engine.Run(t.Context(), supervisor, core.Input(unrenderableInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run control-flow error: %v", err)
	}
	if process.Status() != core.StatusFailed || !errors.Is(process.Failure(), errRenderInput) {
		t.Fatalf("status/failure = %s/%v, want render error", process.Status(), process.Failure())
	}
	if model.calls != 0 {
		t.Fatalf("model calls = %d, want none", model.calls)
	}
}
