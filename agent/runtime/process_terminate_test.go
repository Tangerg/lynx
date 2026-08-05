package runtime_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

type terminatingInput struct{}
type terminatingOutput struct{}

type terminationCapture struct {
	event *event.ProcessTerminated
}

func (*terminationCapture) Name() string { return "termination-capture" }

func (capture *terminationCapture) OnEvent(_ context.Context, published event.Event) {
	if terminated, ok := published.(event.ProcessTerminated); ok {
		capture.event = &terminated
	}
}

func TestProcessContextTerminateStopsProcessWithReason(t *testing.T) {
	capture := new(terminationCapture)
	definition := agent.New(agent.Config{
		Name: "self-terminating",
		Actions: []agent.Action{agent.NewAction(
			"request-stop",
			func(_ context.Context, process *core.ProcessContext, _ terminatingInput) (terminatingOutput, error) {
				return terminatingOutput{}, process.Terminate("work is no longer needed")
			},
			core.ActionConfig{},
		)},
		Goals: []*agent.Goal{agent.NewOutputGoal[terminatingOutput](core.GoalConfig{Description: "output produced"})},
	})
	engine := agent.MustNewEngine(runtime.Config{Extensions: []core.Extension{capture}})

	process, err := engine.Run(t.Context(), definition, core.Input(terminatingInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusTerminated {
		t.Fatalf("status = %s, want terminated", process.Status())
	}
	if capture.event == nil || capture.event.Reason != "work is no longer needed" {
		t.Fatalf("termination event = %#v, want original reason", capture.event)
	}
}
