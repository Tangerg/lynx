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

type dependencyInput struct{ Value int }
type dependencyIntermediate struct{ Value int }
type dependencyOutput struct{ Value int }

func TestEngineProcessActionDependencyScopes(t *testing.T) {
	shared := core.MustDependencyKey[string]("shared")
	actionLocal := core.MustDependencyKey[string]("action-local")

	agentDefinition := agent.New(agent.AgentConfig{Name: "dependency-scopes", Actions: []agent.Action{agent.NewAction("first", func(_ context.Context, processContext *core.ProcessContext, input dependencyInput) (dependencyIntermediate, error) {
		value, err := core.LookupDependency(processContext.Dependencies(), shared)
		if err != nil {
			return dependencyIntermediate{}, err
		}
		if value != "process" {
			return dependencyIntermediate{}, errors.New("process dependency did not shadow engine dependency")
		}
		if err := core.RegisterDependency(processContext.Dependencies(), actionLocal, "first-only"); err != nil {
			return dependencyIntermediate{}, err
		}
		return dependencyIntermediate{Value: input.Value + 1}, nil
	}, core.ActionConfig{}), agent.NewAction("second", func(_ context.Context, processContext *core.ProcessContext, input dependencyIntermediate) (dependencyOutput, error) {
		if _, err := core.LookupDependency(processContext.Dependencies(), actionLocal); !errors.Is(err, core.ErrDependencyNotFound) {
			return dependencyOutput{}, errors.New("action-local dependency leaked into the next action")
		}
		value, err := core.LookupDependency(processContext.Dependencies(), shared)
		if err != nil || value != "process" {
			return dependencyOutput{}, errors.New("process dependency unavailable in second action")
		}
		return dependencyOutput{Value: input.Value + 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[dependencyOutput](core.GoalConfig{Description: "done"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	if err := core.RegisterDependency(engine.Dependencies(), shared, "engine"); err != nil {
		t.Fatalf("RegisterDependency engine: %v", err)
	}
	processDependencies := engine.Dependencies().Child()
	if err := core.RegisterDependency(processDependencies, shared, "process"); err != nil {
		t.Fatalf("RegisterDependency process: %v", err)
	}
	if _, err := engine.Deploy(t.Context(), agentDefinition); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	process, err := engine.Run(
		t.Context(),
		agentDefinition,
		core.Input(dependencyInput{Value: 1}),
		core.ProcessOptions{Dependencies: processDependencies},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, ok := core.Result[dependencyOutput](process)
	if !ok || got.Value != 3 {
		t.Fatalf("result = %#v, %v; want Value=3", got, ok)
	}
	if !engine.Dependencies().Frozen() || !processDependencies.Frozen() {
		t.Fatal("engine and process scopes must freeze before action execution")
	}
	late := core.MustDependencyKey[string]("late")
	if err := core.RegisterDependency(engine.Dependencies(), late, "too-late"); !errors.Is(err, core.ErrDependenciesFrozen) {
		t.Fatalf("late engine registration error = %v", err)
	}
}

func TestProcessDependenciesMustBelongToEngine(t *testing.T) {
	type output struct{ Value int }
	agentDefinition := agent.New(agent.AgentConfig{Name: "dependency-scope-parent", Actions: []agent.Action{agent.NewAction("finish", func(_ context.Context, _ *core.ProcessContext, input dependencyInput) (output, error) {
		return output(input), nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[output](core.GoalConfig{Description: "done"})}})
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(t.Context(), agentDefinition); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	unrelated := core.NewDependencies().Child()
	_, err := engine.Run(
		t.Context(),
		agentDefinition,
		core.Input(dependencyInput{Value: 1}),
		core.ProcessOptions{Dependencies: unrelated},
	)
	if err == nil || !strings.Contains(err.Error(), "immediate child") {
		t.Fatalf("Run error = %v, want scope-parent validation", err)
	}
}
