package workflow_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
)

// Domain types for the loop test. The body is a sub-agent — under
// SpawnChildFresh semantics each iteration runs with a clean blackboard
// seeded only with the typed input, so the body itself cannot read its
// own prior outputs. iteration progress is observable via a closure-
// tracked counter (the realistic shape: a sub-agent whose state lives
// in some external service / instance variable).
type loopIn struct{ Target int }
type loopOut struct{ Value int }

// makeIncrementingBody returns (body, *iterCount). Each invocation
// increments iterCount and returns loopOut{Value: count}. The body
// agent itself is stateless — the counter lives in the closure.
func makeIncrementingBody() (*core.Agent, *int32) {
	var iterCount int32
	body := agent.New("incrementing-body").
		Description("returns loopOut whose Value is the call count").
		Actions(agent.NewAction("step",
			func(_ context.Context, _ *core.ProcessContext, _ loopIn) (loopOut, error) {
				v := atomic.AddInt32(&iterCount, 1)
				return loopOut{Value: int(v)}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[loopOut](core.Goal{Description: "loopOut produced"})).
		Build()
	return body, &iterCount
}

func TestLoopAgent_LoopsUntilUntilTrue(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	body, iterCount := makeIncrementingBody()
	if err := platform.Deploy(body); err != nil {
		t.Fatalf("deploy body: %v", err)
	}

	wf := workflow.LoopAgent[loopIn, loopOut](
		platform,
		workflow.LoopAgentSpec[loopIn, loopOut]{
			Name:          "incr-loop",
			MaxIterations: 10,
			Body:          body,
			Until: func(_ context.Context, in loopIn, last loopOut) bool {
				return last.Value >= in.Target
			},
		},
	)
	if err := platform.Deploy(wf); err != nil {
		t.Fatalf("deploy wf: %v", err)
	}

	proc, err := platform.RunAgent(t.Context(), wf,
		map[string]any{core.DefaultBindingName: loopIn{Target: 4}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[loopOut](proc)
	if !ok {
		t.Fatal("no loopOut bound")
	}
	if got.Value != 4 {
		t.Fatalf("Value = %d, want 4", got.Value)
	}
	if atomic.LoadInt32(iterCount) != 4 {
		t.Fatalf("iterCount = %d, want 4", atomic.LoadInt32(iterCount))
	}
}

func TestLoopAgent_MaxIterationsCapsTheLoop(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	body, iterCount := makeIncrementingBody()
	mustDeploy(t, platform, body)

	wf := workflow.LoopAgent[loopIn, loopOut](
		platform,
		workflow.LoopAgentSpec[loopIn, loopOut]{
			Name:          "capped-loop",
			MaxIterations: 3, // cap kicks in before Target=100
			Body:          body,
			Until: func(_ context.Context, in loopIn, last loopOut) bool {
				return last.Value >= in.Target
			},
		},
	)
	mustDeploy(t, platform, wf)

	proc, _ := platform.RunAgent(t.Context(), wf,
		map[string]any{core.DefaultBindingName: loopIn{Target: 100}},
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, _ := core.ResultOfType[loopOut](proc)
	if got.Value != 3 {
		t.Fatalf("Value = %d, want 3 (MaxIterations cap)", got.Value)
	}
	if atomic.LoadInt32(iterCount) != 3 {
		t.Fatalf("iterCount = %d, want 3 (MaxIterations cap)", atomic.LoadInt32(iterCount))
	}
}

func TestLoopAgent_BranchIsolation(t *testing.T) {
	// Verify the body sub-agent runs with a FRESH blackboard each
	// iteration: it should NOT see prior iterations' loopOut bindings
	// from the LoopAgent's own blackboard. We check this by having the
	// body assert the absence of any prior loopOut on its blackboard.
	platform := agent.NewPlatform(runtime.PlatformConfig{})

	var sawPriorOut atomic.Bool
	body := agent.New("isolation-body").
		Actions(agent.NewAction("step",
			func(_ context.Context, pc *core.ProcessContext, _ loopIn) (loopOut, error) {
				if _, exists := core.Last[loopOut](pc.Blackboard); exists {
					sawPriorOut.Store(true)
				}
				return loopOut{Value: 1}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[loopOut](core.Goal{Description: "loopOut"})).
		Build()
	mustDeploy(t, platform, body)

	wf := workflow.LoopAgent[loopIn, loopOut](
		platform,
		workflow.LoopAgentSpec[loopIn, loopOut]{
			Name:          "isolation-loop",
			MaxIterations: 3,
			Body:          body,
			Until:         func(context.Context, loopIn, loopOut) bool { return false },
		},
	)
	mustDeploy(t, platform, wf)

	platform.RunAgent(t.Context(), wf,
		map[string]any{core.DefaultBindingName: loopIn{Target: 100}},
		core.ProcessOptions{},
	)
	if sawPriorOut.Load() {
		t.Fatal("body saw a prior iteration's loopOut on its blackboard — branch isolation broken")
	}
}

func TestLoopAgent_PanicsOnNilBody(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	workflow.LoopAgent[loopIn, loopOut](platform, workflow.LoopAgentSpec[loopIn, loopOut]{
		Name:  "no-body",
		Until: func(_ context.Context, _ loopIn, _ loopOut) bool { return true },
	})
}

func TestLoopAgent_PanicsOnNilUntil(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	body, _ := makeIncrementingBody()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	workflow.LoopAgent[loopIn, loopOut](platform, workflow.LoopAgentSpec[loopIn, loopOut]{
		Name: "no-until",
		Body: body,
	})
}
