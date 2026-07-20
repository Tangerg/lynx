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
// RunChildIsolated semantics each iteration runs with a clean blackboard
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
	body := agent.New(agent.AgentConfig{Name: "incrementing-body", Description: "returns loopOut whose Value is the call count", Actions: []agent.Action{agent.NewAction("step", func(_ context.Context, _ *core.ProcessContext, _ loopIn) (loopOut, error) {
		v := atomic.AddInt32(&iterCount, 1)
		return loopOut{Value: int(v)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[loopOut](core.GoalConfig{Description: "loopOut produced"})}})
	return body, &iterCount
}

func TestLoop_LoopsUntilUntilTrue(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	body, iterCount := makeIncrementingBody()
	if _, err := engine.Deploy(body); err != nil {
		t.Fatalf("deploy body: %v", err)
	}

	wf, err := workflow.Loop[loopIn, loopOut](
		engine,
		workflow.LoopConfig[loopIn, loopOut]{
			Name:          "incr-loop",
			MaxIterations: 10,
			Body:          body,
			Until: func(_ context.Context, in loopIn, last loopOut) bool {
				return last.Value >= in.Target
			},
		},
	)
	if err != nil {
		t.Fatalf("Loop: %v", err)
	}
	if _, err := engine.Deploy(wf); err != nil {
		t.Fatalf("deploy wf: %v", err)
	}

	proc, runErr := engine.Run(t.Context(), wf,
		core.Input(loopIn{Target: 4}),
		core.ProcessOptions{},
	)
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.Result[loopOut](proc)
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

func TestLoop_MaxIterationsCapsTheLoop(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	body, iterCount := makeIncrementingBody()
	mustDeploy(t, engine, body)

	wf, err := workflow.Loop[loopIn, loopOut](
		engine,
		workflow.LoopConfig[loopIn, loopOut]{
			Name:          "capped-loop",
			MaxIterations: 3, // cap kicks in before Target=100
			Body:          body,
			Until: func(_ context.Context, in loopIn, last loopOut) bool {
				return last.Value >= in.Target
			},
		},
	)
	if err != nil {
		t.Fatalf("Loop: %v", err)
	}
	mustDeploy(t, engine, wf)

	proc, _ := engine.Run(t.Context(), wf,
		core.Input(loopIn{Target: 100}),
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, _ := core.Result[loopOut](proc)
	if got.Value != 3 {
		t.Fatalf("Value = %d, want 3 (MaxIterations cap)", got.Value)
	}
	if atomic.LoadInt32(iterCount) != 3 {
		t.Fatalf("iterCount = %d, want 3 (MaxIterations cap)", atomic.LoadInt32(iterCount))
	}
}

func TestLoop_BranchIsolation(t *testing.T) {
	// Verify the body sub-agent runs with a FRESH blackboard each
	// iteration: it should NOT see prior iterations' loopOut bindings
	// from the Loop's own blackboard. We check this by having the
	// body assert the absence of any prior loopOut on its blackboard.
	engine := agent.MustNewEngine(runtime.Config{})

	var sawPriorOut atomic.Bool
	body := agent.New(agent.AgentConfig{Name: "isolation-body", Actions: []agent.Action{agent.NewAction("step", func(_ context.Context, pc *core.ProcessContext, _ loopIn) (loopOut, error) {
		if _, exists := core.Last[loopOut](pc.Blackboard()); exists {
			sawPriorOut.Store(true)
		}
		return loopOut{Value: 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[loopOut](core.GoalConfig{Description: "loopOut"})}})
	mustDeploy(t, engine, body)

	wf, err := workflow.Loop[loopIn, loopOut](
		engine,
		workflow.LoopConfig[loopIn, loopOut]{
			Name:          "isolation-loop",
			MaxIterations: 3,
			Body:          body,
			Until:         func(context.Context, loopIn, loopOut) bool { return false },
		},
	)
	if err != nil {
		t.Fatalf("Loop: %v", err)
	}
	mustDeploy(t, engine, wf)

	engine.Run(t.Context(), wf,
		core.Input(loopIn{Target: 100}),
		core.ProcessOptions{},
	)
	if sawPriorOut.Load() {
		t.Fatal("body saw a prior iteration's loopOut on its blackboard — branch isolation broken")
	}
}

func TestLoop_RejectsNilBody(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := workflow.Loop[loopIn, loopOut](engine, workflow.LoopConfig[loopIn, loopOut]{
		Name:  "no-body",
		Until: func(_ context.Context, _ loopIn, _ loopOut) bool { return true },
	}); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoop_RejectsNilUntil(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	body, _ := makeIncrementingBody()
	if _, err := workflow.Loop[loopIn, loopOut](engine, workflow.LoopConfig[loopIn, loopOut]{
		Name: "no-until",
		Body: body,
	}); err == nil {
		t.Fatal("expected error")
	}
}
