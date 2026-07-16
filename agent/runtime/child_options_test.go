package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

type childPolicyInput struct{}
type childPolicyOutput struct{ Path string }

var childPolicyKey = core.MustDependencyKey[string]("runtime_test.child_policy")

func TestChildOptionsApplyToTheWholeDelegationTree(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})

	leaf := agent.New(agent.AgentConfig{
		Name: "policy-leaf",
		Actions: []agent.Action{agent.NewAction("read-policy", func(_ context.Context, pc *core.ProcessContext, _ childPolicyInput) (childPolicyOutput, error) {
			value, err := core.LookupDependency(pc.Dependencies(), childPolicyKey)
			if err != nil {
				return childPolicyOutput{}, err
			}
			return childPolicyOutput{Path: value}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[childPolicyOutput](core.GoalConfig{Description: "leaf done"})},
	})
	leafDeployment, err := engine.Deploy(leaf)
	if err != nil {
		t.Fatalf("deploy leaf: %v", err)
	}

	middle := agent.New(agent.AgentConfig{
		Name: "policy-middle",
		Actions: []agent.Action{agent.NewAction("run-leaf", func(ctx context.Context, pc *core.ProcessContext, _ childPolicyInput) (childPolicyOutput, error) {
			middlePolicy, err := core.LookupDependency(pc.Dependencies(), childPolicyKey)
			if err != nil {
				return childPolicyOutput{}, err
			}
			child, err := engine.RunChild(ctx, leafDeployment, childPolicyInput{})
			if err != nil {
				return childPolicyOutput{}, err
			}
			leafOutput, ok := core.Result[childPolicyOutput](child)
			if !ok {
				return childPolicyOutput{}, errors.New("leaf produced no output")
			}
			return childPolicyOutput{Path: middlePolicy + ">" + leafOutput.Path}, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[childPolicyOutput](core.GoalConfig{Description: "middle done"})},
	})
	middleDeployment, err := engine.Deploy(middle)
	if err != nil {
		t.Fatalf("deploy middle: %v", err)
	}

	root := agent.New(agent.AgentConfig{
		Name: "policy-root",
		Actions: []agent.Action{agent.NewAction("run-middle", func(ctx context.Context, _ *core.ProcessContext, _ childPolicyInput) (childPolicyOutput, error) {
			child, err := engine.RunChild(ctx, middleDeployment, childPolicyInput{})
			if err != nil {
				return childPolicyOutput{}, err
			}
			output, ok := core.Result[childPolicyOutput](child)
			if !ok {
				return childPolicyOutput{}, errors.New("middle produced no output")
			}
			return output, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[childPolicyOutput](core.GoalConfig{Description: "root done"})},
	})
	if _, err := engine.Deploy(root); err != nil {
		t.Fatalf("deploy root: %v", err)
	}

	var (
		mu         sync.Mutex
		configured []string
	)
	process, err := engine.Run(t.Context(), root, map[string]any{core.DefaultBindingName: childPolicyInput{}}, core.ProcessOptions{
		ChildOptions: func(_ context.Context, _ core.ProcessView, child *core.Agent) (core.ProcessOptions, error) {
			dependencies := engine.Dependencies().Child()
			if err := core.RegisterDependency(dependencies, childPolicyKey, child.Name()); err != nil {
				return core.ProcessOptions{}, err
			}
			mu.Lock()
			configured = append(configured, child.Name())
			mu.Unlock()
			return core.ProcessOptions{Dependencies: dependencies}, nil
		},
	})
	if err != nil {
		t.Fatalf("Run root: %v", err)
	}
	output, ok := core.Result[childPolicyOutput](process)
	if !ok || output.Path != "policy-middle>policy-leaf" {
		t.Fatalf("root output = %+v ok=%v, want policy-middle>policy-leaf", output, ok)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(configured) != 2 || configured[0] != "policy-middle" || configured[1] != "policy-leaf" {
		t.Fatalf("configured children = %v, want [policy-middle policy-leaf]", configured)
	}
}

func TestKillCancelsRunningChildTree(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	started := make(chan struct{})
	exited := make(chan struct{})

	leaf := agent.New(agent.AgentConfig{
		Name: "cancel-leaf",
		Actions: []agent.Action{agent.NewAction("block", func(ctx context.Context, _ *core.ProcessContext, _ childPolicyInput) (childPolicyOutput, error) {
			close(started)
			<-ctx.Done()
			close(exited)
			return childPolicyOutput{}, ctx.Err()
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[childPolicyOutput](core.GoalConfig{Description: "leaf done"})},
	})
	leafDeployment, err := engine.Deploy(leaf)
	if err != nil {
		t.Fatalf("deploy leaf: %v", err)
	}

	root := agent.New(agent.AgentConfig{
		Name: "cancel-root",
		Actions: []agent.Action{agent.NewAction("run-leaf", func(ctx context.Context, _ *core.ProcessContext, _ childPolicyInput) (childPolicyOutput, error) {
			child, err := engine.RunChild(ctx, leafDeployment, childPolicyInput{})
			if err != nil {
				return childPolicyOutput{}, err
			}
			output, _ := core.Result[childPolicyOutput](child)
			return output, nil
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[childPolicyOutput](core.GoalConfig{Description: "root done"})},
	})
	if _, err := engine.Deploy(root); err != nil {
		t.Fatalf("deploy root: %v", err)
	}

	process, done := engine.Start(t.Context(), root, map[string]any{core.DefaultBindingName: childPolicyInput{}}, core.ProcessOptions{})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("child never started")
	}

	var child *runtime.Process
	for _, candidate := range engine.Processes() {
		if candidate.ParentID() == process.ID() {
			child = candidate
			break
		}
	}
	if child == nil {
		t.Fatal("running child was not registered")
	}
	if err := engine.Kill(process.ID()); err != nil {
		t.Fatalf("Kill root: %v", err)
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("root done error = %v, want nil or context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("root did not stop after Kill")
	}
	select {
	case <-exited:
	case <-time.After(time.Second):
		t.Fatal("child action context was not canceled")
	}
	if process.Status() != core.StatusKilled || child.Status() != core.StatusKilled {
		t.Fatalf("statuses root=%s child=%s, want killed/killed", process.Status(), child.Status())
	}
}
