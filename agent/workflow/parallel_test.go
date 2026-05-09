package workflow_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
func makeScoringAgent(name string, score int, inFlight *int32) *core.Agent {
	return agent.New(name).
		Description(fmt.Sprintf("scoring agent yielding %d", score)).
		Actions(agent.NewAction("score",
			func(_ context.Context, _ *core.ProcessContext, _ paIn) (paScore, error) {
				if inFlight != nil {
					atomic.AddInt32(inFlight, 1)
					defer atomic.AddInt32(inFlight, -1)
					time.Sleep(15 * time.Millisecond)
				}
				return paScore{Value: score}, nil
			},
			core.ActionConfig{},
		)).
		Goals(agent.GoalProducing[paScore](core.Goal{Description: "score produced"})).
		Build()
}

func TestParallelAgents_RunsAllAndJoins(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})

	var inFlight int32
	a1 := makeScoringAgent("scorer-1", 1, &inFlight)
	a2 := makeScoringAgent("scorer-2", 2, &inFlight)
	a3 := makeScoringAgent("scorer-3", 3, &inFlight)
	for _, a := range []*core.Agent{a1, a2, a3} {
		if err := platform.Deploy(a); err != nil {
			t.Fatalf("deploy %s: %v", a.Name, err)
		}
	}

	wf := workflow.ParallelAgents[paIn, paScore, paSummary](
		platform,
		workflow.ParallelAgentsSpec[paIn, paScore, paSummary]{
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
	if err := platform.Deploy(wf); err != nil {
		t.Fatalf("deploy wf: %v", err)
	}

	proc, err := platform.RunAgent(t.Context(), wf,
		map[string]any{core.DefaultBindingName: paIn{Topic: "x"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}

	got, ok := core.ResultOfType[paSummary](proc)
	if !ok {
		t.Fatal("no paSummary bound")
	}
	if got.Total != 1+2+3 {
		t.Fatalf("Total = %d, want 6", got.Total)
	}
}

func TestParallelAgents_SubAgentFailureCancels(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})

	good := makeScoringAgent("good", 5, nil)
	bad := agent.New("bad").
		Actions(agent.NewAction("fail",
			func(_ context.Context, _ *core.ProcessContext, _ paIn) (paScore, error) {
				return paScore{}, errors.New("kaboom")
			},
			core.ActionConfig{QoS: core.ActionQoS{MaxAttempts: 1}},
		)).
		Goals(agent.GoalProducing[paScore](core.Goal{Description: "score (will fail)"})).
		Build()
	mustDeploy(t, platform, good, bad)

	wf := workflow.ParallelAgents[paIn, paScore, paSummary](
		platform,
		workflow.ParallelAgentsSpec[paIn, paScore, paSummary]{
			Name:   "parallel-fail",
			Agents: []*core.Agent{good, bad},
			Joiner: func(_ context.Context, _ *core.ProcessContext, items []paScore) (paSummary, error) {
				return paSummary{Total: items[0].Value + items[1].Value}, nil
			},
		},
	)
	mustDeploy(t, platform, wf)

	proc, _ := platform.RunAgent(t.Context(), wf,
		map[string]any{core.DefaultBindingName: paIn{Topic: "x"}},
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusFailed {
		t.Fatalf("status = %s; want StatusFailed", proc.Status())
	}
	if failure := proc.Failure(); failure == nil || !strings.Contains(failure.Error(), "kaboom") {
		t.Fatalf("failure = %v; want one mentioning 'kaboom'", failure)
	}
}

func TestParallelAgents_MaxConcurrencyCaps(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})

	var inFlight int32
	var peak int32
	updatePeak := func(now int32) {
		for {
			cur := atomic.LoadInt32(&peak)
			if now <= cur || atomic.CompareAndSwapInt32(&peak, cur, now) {
				return
			}
		}
	}
	mkProbed := func(name string, score int) *core.Agent {
		return agent.New(name).
			Actions(agent.NewAction("score",
				func(_ context.Context, _ *core.ProcessContext, _ paIn) (paScore, error) {
					now := atomic.AddInt32(&inFlight, 1)
					updatePeak(now)
					defer atomic.AddInt32(&inFlight, -1)
					time.Sleep(20 * time.Millisecond)
					return paScore{Value: score}, nil
				},
				core.ActionConfig{},
			)).
			Goals(agent.GoalProducing[paScore](core.Goal{Description: "score produced"})).
			Build()
	}
	subs := make([]*core.Agent, 4)
	for i := range subs {
		subs[i] = mkProbed(fmt.Sprintf("probed-%d", i), i+1)
	}
	mustDeploy(t, platform, subs...)

	wf := workflow.ParallelAgents[paIn, paScore, paSummary](
		platform,
		workflow.ParallelAgentsSpec[paIn, paScore, paSummary]{
			Name:           "capped",
			MaxConcurrency: 2,
			Agents:         subs,
			Joiner: func(_ context.Context, _ *core.ProcessContext, items []paScore) (paSummary, error) {
				return paSummary{}, nil
			},
		},
	)
	mustDeploy(t, platform, wf)

	proc, err := platform.RunAgent(t.Context(), wf,
		map[string]any{core.DefaultBindingName: paIn{Topic: "x"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	if peak > 2 {
		t.Fatalf("peak in-flight = %d, expected ≤ 2 (MaxConcurrency)", peak)
	}
}

func TestParallelAgents_PanicsOnEmptyAgents(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	workflow.ParallelAgents[paIn, paScore, paSummary](
		platform,
		workflow.ParallelAgentsSpec[paIn, paScore, paSummary]{
			Name: "empty",
			Joiner: func(_ context.Context, _ *core.ProcessContext, _ []paScore) (paSummary, error) {
				return paSummary{}, nil
			},
		},
	)
}

func TestParallelAgents_PanicsOnNilJoiner(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	workflow.ParallelAgents[paIn, paScore, paSummary](
		platform,
		workflow.ParallelAgentsSpec[paIn, paScore, paSummary]{
			Name:   "no-joiner",
			Agents: []*core.Agent{makeScoringAgent("a", 1, nil)},
		},
	)
}
