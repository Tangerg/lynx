package workflow_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

func invalidWorkflowAgent() *core.Agent {
	action := core.NewAction[struct{}, struct{}]("invalid-workflow-action", func(context.Context, *core.ProcessContext, struct{}) (struct{}, error) {
		return struct{}{}, nil
	}, core.ActionConfig{})
	return core.NewAgent(core.AgentConfig{
		Name:    " invalid-workflow-agent ",
		Actions: []core.Action{action},
		Goals:   []*core.Goal{core.NewOutputGoal[struct{}](core.GoalConfig{Name: "invalid-workflow-goal"})},
	})
}

// mustDeploy deploys agents on engine and fails the test on the first error.
func mustDeploy(t *testing.T, p *runtime.Engine, agents ...*core.Agent) {
	t.Helper()
	for _, a := range agents {
		if _, err := p.Deploy(t.Context(), a); err != nil {
			t.Fatalf("deploy %q: %v", a.Name(), err)
		}
	}
}

func mustRun(t *testing.T, engine *runtime.Engine, definition *core.Agent, input core.Bindings) *runtime.Process {
	t.Helper()
	process, err := engine.Run(t.Context(), definition, input, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("run %q: %v", definition.Name(), err)
	}
	if process == nil {
		t.Fatalf("run %q returned nil without an error", definition.Name())
	}
	return process
}

func mustResult[T any](t *testing.T, process core.ProcessView) T {
	t.Helper()
	result, ok := core.Result[T](process)
	if !ok {
		t.Fatalf("process %q has no result of the requested type", process.ID())
	}
	return result
}
