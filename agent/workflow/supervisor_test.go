package workflow_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
	"github.com/Tangerg/lynx/chatclient"
	"github.com/Tangerg/lynx/core/chat"
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

func makeSubAgent() *core.Agent {
	return agent.New(agent.AgentConfig{Name: "worker", Actions: []agent.Action{agent.NewAction("work", func(_ context.Context, _ *core.ProcessContext, in supTopic) (supAnswer, error) {
		return supAnswer{Text: "did " + in.Title}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[supAnswer](core.GoalConfig{Name: "worker-goal", Tool: core.NewGoalTool[supTopic](core.GoalToolConfig{Description: "do work on a topic"})})}})
}

func TestSupervisor_Validation(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, makeSubAgent())

	parse := func(s string) (supAnswer, error) { return supAnswer{Text: s}, nil }

	cases := []struct {
		name   string
		config workflow.SupervisorConfig[supTopic, supAnswer]
	}{
		{"empty name", workflow.SupervisorConfig[supTopic, supAnswer]{Agents: []string{"worker"}, Parse: parse}},
		{"no agents", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Parse: parse}},
		{"nil parse", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Agents: []string{"worker"}}},
		{"unknown agent", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Agents: []string{"ghost"}, Parse: parse}},
	}
	for _, test := range cases {
		if _, err := workflow.Supervisor(engine, test.config); err == nil {
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
	mustDeploy(t, engine, makeSubAgent())

	supervisor, err := workflow.Supervisor(engine, workflow.SupervisorConfig[supTopic, supAnswer]{
		Name:         "supervisor",
		Description:  "orchestrate the worker",
		Agents:       []string{"worker"},
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
