package workflow_test

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
	"github.com/Tangerg/lynx/core/model/chat"
)

// fakeModel is a minimal chat.Model that always replies with a fixed text,
// enough to drive the supervisor's chat tool loop to a final answer without
// a real provider.
type fakeModel struct{ text string }

func (fakeModel) DefaultOptions() chat.Options    { return chat.Options{} }
func (fakeModel) Metadata() chat.ModelMetadata    { return chat.ModelMetadata{} }
func (m fakeModel) Call(context.Context, *chat.Request) (*chat.Response, error) {
	return fakeTextResponse(m.text), nil
}
func (m fakeModel) Stream(context.Context, *chat.Request) iter.Seq2[*chat.Response, error] {
	return func(yield func(*chat.Response, error) bool) { yield(fakeTextResponse(m.text), nil) }
}

func fakeTextResponse(text string) *chat.Response {
	resp, _ := chat.NewResponse(
		&chat.Result{
			AssistantMessage: chat.NewAssistantMessage(text),
			Metadata:         &chat.ResultMetadata{FinishReason: chat.FinishReasonStop},
		},
		&chat.ResponseMetadata{},
	)
	return resp
}

type supTopic struct{ Title string }
type supAnswer struct{ Text string }

func makeSubAgent() *core.Agent {
	return agent.New("worker").
		Actions(agent.NewAction("work",
			func(_ context.Context, _ *core.ProcessContext, in supTopic) (supAnswer, error) {
				return supAnswer{Text: "did " + in.Title}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[supAnswer](core.Goal{
			Name: "worker-goal",
			Export: &core.GoalExport{
				Description: "do work on a topic",
				InputSample: supTopic{},
			},
		})).
		Build()
}

func TestSupervisor_Validation(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	mustDeploy(t, platform, makeSubAgent())

	parse := func(s string) (supAnswer, error) { return supAnswer{Text: s}, nil }

	cases := []struct {
		name string
		cfg  workflow.SupervisorConfig[supTopic, supAnswer]
	}{
		{"empty name", workflow.SupervisorConfig[supTopic, supAnswer]{Subagents: []string{"worker"}, Parse: parse}},
		{"no subagents", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Parse: parse}},
		{"nil parse", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Subagents: []string{"worker"}}},
		{"unknown subagent", workflow.SupervisorConfig[supTopic, supAnswer]{Name: "s", Subagents: []string{"ghost"}, Parse: parse}},
	}
	for _, tc := range cases {
		if _, err := workflow.Supervisor(platform, tc.cfg); err == nil {
			t.Errorf("%s: expected error, got nil", tc.name)
		}
	}
}

// TestSupervisor_EndToEnd drives the supervisor with a fake model that
// returns a final answer directly, confirming the chat client wiring and
// Parse path produce the typed output.
func TestSupervisor_EndToEnd(t *testing.T) {
	client, err := chat.NewClient(fakeModel{text: "orchestrated result"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	platform := agent.NewPlatform(runtime.PlatformConfig{ChatClient: client})
	mustDeploy(t, platform, makeSubAgent())

	sup, err := workflow.Supervisor(platform, workflow.SupervisorConfig[supTopic, supAnswer]{
		Name:         "supervisor",
		Description:  "orchestrate the worker",
		Subagents:    []string{"worker"},
		Instructions: "Use the worker tool, then reply.",
		Parse:        func(text string) (supAnswer, error) { return supAnswer{Text: text}, nil },
	})
	if err != nil {
		t.Fatalf("Supervisor: %v", err)
	}
	mustDeploy(t, platform, sup)

	proc, err := platform.RunAgent(context.Background(), sup,
		map[string]any{core.DefaultBindingName: supTopic{Title: "go generics"}},
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure=%v", proc.Status(), proc.Failure())
	}

	out, ok := core.ResultOfType[supAnswer](proc)
	if !ok {
		t.Fatal("no supAnswer produced")
	}
	if out.Text != "orchestrated result" {
		t.Fatalf("output = %q, want %q", out.Text, "orchestrated result")
	}
}
