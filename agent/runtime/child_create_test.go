package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type panickingChildExtension struct{ cause error }

func (e panickingChildExtension) Name() string { panic(e.cause) }

func TestRunChildReturnsExtensionNamePanic(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}

	parentDef := agent.New(agent.AgentConfig{Name: "parent-extension-check", Actions: []agent.Action{agent.NewAction("delegate", func(ctx context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
		_, err := engine.RunChild(ctx, childDeployment, input)
		return parentOutput{}, err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	cause := errors.New("child extension identity unavailable")
	parent, err := engine.Run(t.Context(), parentDef, core.Input(subInput{Value: 1}), core.ProcessOptions{
		ChildOptions: func(context.Context, core.ProcessView, *core.Agent) (core.ProcessOptions, error) {
			return core.ProcessOptions{Extensions: []core.Extension{panickingChildExtension{cause: cause}}}, nil
		},
	})
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}
	if parent.Status() != core.StatusFailed || !errors.Is(parent.Failure(), cause) {
		t.Fatalf("parent status=%s failure=%v, want extension panic", parent.Status(), parent.Failure())
	}
}

func TestRunChildRejectsASecondTreeBudget(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatalf("deploy child: %v", err)
	}
	parentDef := agent.New(agent.AgentConfig{Name: "parent-budget-check", Actions: []agent.Action{agent.NewAction("delegate", func(ctx context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
		_, err := engine.RunChild(ctx, childDeployment, input)
		return parentOutput{}, err
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})

	parent, err := engine.Run(t.Context(), parentDef, core.Input(subInput{Value: 1}), core.ProcessOptions{
		ChildOptions: func(context.Context, core.ProcessView, *core.Agent) (core.ProcessOptions, error) {
			return core.ProcessOptions{Budget: core.Budget{ActionLimit: 1}}, nil
		},
	})
	if err != nil {
		t.Fatalf("run parent: %v", err)
	}
	failure := parent.Failure()
	if parent.Status() != core.StatusFailed || failure == nil ||
		!strings.Contains(failure.Error(), "child budget must be configured on the root") {
		t.Fatalf("parent status=%s failure=%v, want child-budget rejection", parent.Status(), parent.Failure())
	}
}

func TestRunChildRejectsInactiveParent(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	childDeployment, err := engine.Deploy(t.Context(), childAgent())
	if err != nil {
		t.Fatal(err)
	}
	parentDef := agent.New(agent.AgentConfig{Name: "inactive-parent", Actions: []agent.Action{agent.NewAction("finish", func(_ context.Context, _ *core.ProcessContext, input subInput) (parentOutput, error) {
		return parentOutput{Final: input.Value}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[parentOutput](core.GoalConfig{Description: "done"})}})
	parent, err := engine.Run(t.Context(), parentDef, core.Input(subInput{Value: 1}), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	before := len(engine.Processes())
	_, err = engine.RunChild(core.WithProcessView(t.Context(), parent), childDeployment, subInput{Value: 2})
	if !errors.Is(err, runtime.ErrChildParentInactive) {
		t.Fatalf("RunChild error = %v, want ErrChildParentInactive", err)
	}
	if got := len(engine.Processes()); got != before {
		t.Fatalf("registered process count = %d, want %d", got, before)
	}
}
