package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
)

// Domain types for the parallel test: each agent independently scores
// the same input.
type paIn struct{ Topic string }
type paScore struct{ Value int }
type paSummary struct{ Total int }

// makeScoringAgent: takes paIn, produces paScore. The score is parameterized
// so we can build N distinct agents that produce different values.
func makeScoringAgent(name string, score int) *core.Agent {
	return agent.New(agent.AgentConfig{Name: name, Description: fmt.Sprintf("scoring agent yielding %d", score), Actions: []agent.Action{agent.NewAction("score", func(_ context.Context, _ *core.ProcessContext, _ paIn) (paScore, error) {
		return paScore{Value: score}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[paScore](core.GoalConfig{Description: "score produced"})}})
}

func TestParallel_RunsAllAndJoins(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})

	a1 := makeScoringAgent("scorer-1", 1)
	a2 := makeScoringAgent("scorer-2", 2)
	a3 := makeScoringAgent("scorer-3", 3)
	for _, a := range []*core.Agent{a1, a2, a3} {
		if _, err := engine.Deploy(a); err != nil {
			t.Fatalf("deploy %s: %v", a.Name(), err)
		}
	}

	wf, err := workflow.Parallel[paIn, paScore, paSummary](
		engine,
		workflow.ParallelConfig[paIn, paScore, paSummary]{
			Name:   "parallel-scoring",
			Agents: []*core.Agent{a1, a2, a3},
			Joiner: func(_ context.Context, _ *core.ProcessContext, items []paScore) (paSummary, error) {
				sum := 0
				for _, s := range items {
					sum += s.Value
				}
				return paSummary{Total: sum}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("Parallel: %v", err)
	}
	_, err = engine.Deploy(wf)
	if err != nil {
		t.Fatalf("deploy wf: %v", err)
	}

	proc, err := engine.Run(t.Context(), wf,
		core.Input(paIn{Topic: "x"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}

	got, ok := core.Result[paSummary](proc)
	if !ok {
		t.Fatal("no paSummary bound")
	}
	if got.Total != 1+2+3 {
		t.Fatalf("Total = %d, want 6", got.Total)
	}
}

func TestParallel_SubAgentFailureCancels(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})

	good := makeScoringAgent("good", 5)
	bad := agent.New(agent.AgentConfig{Name: "bad", Actions: []agent.Action{agent.NewAction("fail", func(_ context.Context, _ *core.ProcessContext, _ paIn) (paScore, error) {
		return paScore{}, errors.New("kaboom")
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[paScore](core.GoalConfig{Description: "score (will fail)"})}})
	mustDeploy(t, engine, good, bad)

	wf, err := workflow.Parallel[paIn, paScore, paSummary](
		engine,
		workflow.ParallelConfig[paIn, paScore, paSummary]{
			Name:   "parallel-fail",
			Agents: []*core.Agent{good, bad},
			Joiner: func(_ context.Context, _ *core.ProcessContext, items []paScore) (paSummary, error) {
				return paSummary{Total: items[0].Value + items[1].Value}, nil
			},
		},
	)
	if err != nil {
		t.Fatalf("Parallel: %v", err)
	}
	mustDeploy(t, engine, wf)

	proc, _ := engine.Run(t.Context(), wf,
		core.Input(paIn{Topic: "x"}),
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusFailed {
		t.Fatalf("status = %s; want StatusFailed", proc.Status())
	}
	if failure := proc.Failure(); failure == nil || !strings.Contains(failure.Error(), "kaboom") {
		t.Fatalf("failure = %v; want one mentioning 'kaboom'", failure)
	}
}

func TestParallel_MaxConcurrencyCaps(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		engine := agent.MustNewEngine(runtime.Config{})

		var inFlight int32
		var peak int32
		started := make(chan struct{}, 4)
		release := make(chan struct{})
		released := false
		defer func() {
			if !released {
				close(release)
			}
		}()
		updatePeak := func(now int32) {
			for {
				cur := atomic.LoadInt32(&peak)
				if now <= cur || atomic.CompareAndSwapInt32(&peak, cur, now) {
					return
				}
			}
		}
		mkProbed := func(name string, score int) *core.Agent {
			return agent.New(agent.AgentConfig{Name: name, Actions: []agent.Action{agent.NewAction("score", func(ctx context.Context, _ *core.ProcessContext, _ paIn) (paScore, error) {
				now := atomic.AddInt32(&inFlight, 1)
				updatePeak(now)
				defer atomic.AddInt32(&inFlight, -1)
				started <- struct{}{}
				select {
				case <-release:
					return paScore{Value: score}, nil
				case <-ctx.Done():
					return paScore{}, ctx.Err()
				}
			}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[paScore](core.GoalConfig{Description: "score produced"})}})
		}
		subs := make([]*core.Agent, 4)
		for i := range subs {
			subs[i] = mkProbed(fmt.Sprintf("probed-%d", i), i+1)
		}
		mustDeploy(t, engine, subs...)

		wf, err := workflow.Parallel[paIn, paScore, paSummary](
			engine,
			workflow.ParallelConfig[paIn, paScore, paSummary]{
				Name:           "capped",
				MaxConcurrency: 2,
				Agents:         subs,
				Joiner: func(_ context.Context, _ *core.ProcessContext, items []paScore) (paSummary, error) {
					return paSummary{}, nil
				},
			},
		)
		if err != nil {
			t.Fatalf("Parallel: %v", err)
		}
		mustDeploy(t, engine, wf)

		type runResult struct {
			process *runtime.Process
			err     error
		}
		done := make(chan runResult, 1)
		go func() {
			process, runErr := engine.Run(t.Context(), wf,
				core.Input(paIn{Topic: "x"}),
				core.ProcessOptions{},
			)
			done <- runResult{process: process, err: runErr}
		}()

		// Wait for the allowed pair, then let synctest prove every goroutine is
		// durably blocked. If the cap were ignored, the remaining two actions
		// would also have reached started before Wait returned.
		<-started
		<-started
		synctest.Wait()
		if extra := len(started); extra != 0 {
			t.Fatalf("%d action(s) exceeded MaxConcurrency before release", extra)
		}
		if got := atomic.LoadInt32(&peak); got != 2 {
			t.Fatalf("peak in-flight = %d, want exactly 2", got)
		}

		close(release)
		released = true
		result := <-done
		if result.err != nil {
			t.Fatalf("Run: %v", result.err)
		}
		if result.process.Status() != core.StatusCompleted {
			t.Fatalf("status = %s; failure = %v", result.process.Status(), result.process.Failure())
		}
	})
}

func TestParallel_RejectsEmptyAgents(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := workflow.Parallel[paIn, paScore, paSummary](
		engine,
		workflow.ParallelConfig[paIn, paScore, paSummary]{
			Name: "empty",
			Joiner: func(_ context.Context, _ *core.ProcessContext, _ []paScore) (paSummary, error) {
				return paSummary{}, nil
			},
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParallel_RejectsNilJoiner(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	_, err := workflow.Parallel[paIn, paScore, paSummary](
		engine,
		workflow.ParallelConfig[paIn, paScore, paSummary]{
			Name:   "no-joiner",
			Agents: []*core.Agent{makeScoringAgent("a", 1)},
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
}
