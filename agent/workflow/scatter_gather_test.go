package workflow_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
)

type sgIn struct{ Topic string }
type sgElement struct{ Score int }
type sgResult struct{ Total int }

func TestScatterGather_RunsAllGeneratorsAndJoins(t *testing.T) {
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	var active atomic.Int32
	var peak atomic.Int32
	gen := func(score int) func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
		return func(ctx context.Context, _ *core.ProcessContext, _ sgIn) (sgElement, error) {
			now := active.Add(1)
			defer active.Add(-1)
			for current := peak.Load(); now > current && !peak.CompareAndSwap(current, now); current = peak.Load() {
			}
			started <- struct{}{}
			select {
			case <-release:
				return sgElement{Score: score}, nil
			case <-ctx.Done():
				return sgElement{}, ctx.Err()
			}
		}
	}

	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name:        "fanout",
		Description: "score-fanout test",
		Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
			gen(1), gen(2), gen(3),
		},
		Joiner: func(_ context.Context, _ *core.ProcessContext, items []sgElement) (sgResult, error) {
			sum := 0
			for _, e := range items {
				sum += e.Score
			}
			return sgResult{Total: sum}, nil
		},
	})
	if err != nil {
		t.Fatalf("ScatterGather: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{})
	_, err = engine.Deploy(a)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	type runResult struct {
		process *runtime.Process
		err     error
	}
	done := make(chan runResult, 1)
	go func() {
		proc, runErr := engine.Run(t.Context(), a,
			core.Input(sgIn{Topic: "test"}),
			core.ProcessOptions{},
		)
		done <- runResult{process: proc, err: runErr}
	}()
	for range 3 {
		select {
		case <-started:
		case result := <-done:
			close(release)
			t.Fatalf("Run exited before all branches entered: %v", result.err)
		case <-t.Context().Done():
			close(release)
			t.Fatal(t.Context().Err())
		}
	}
	if got := peak.Load(); got != 3 {
		close(release)
		t.Fatalf("parallel peak = %d, want 3", got)
	}
	close(release)
	result := <-done
	if result.err != nil {
		t.Fatalf("Run: %v", result.err)
	}
	proc := result.process
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.Result[sgResult](proc)
	if !ok {
		t.Fatal("no sgResult bound")
	}
	if got.Total != 1+2+3 {
		t.Fatalf("Total = %d, want 6", got.Total)
	}
}

func TestScatterGather_FirstErrorCancelsAndJoinsOtherBranches(t *testing.T) {
	blockingStarted := make(chan struct{})
	blockingExited := make(chan struct{})
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name: "fanout-cancel",
		Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
			func(ctx context.Context, _ *core.ProcessContext, _ sgIn) (sgElement, error) {
				close(blockingStarted)
				<-ctx.Done()
				close(blockingExited)
				return sgElement{}, ctx.Err()
			},
			func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
				<-blockingStarted
				return sgElement{}, errors.New("boom")
			},
		},
		Joiner: func(_ context.Context, _ *core.ProcessContext, items []sgElement) (sgResult, error) {
			return sgResult{Total: len(items)}, nil
		},
	})
	if err != nil {
		t.Fatalf("ScatterGather: %v", err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, _ := engine.Run(t.Context(), a,
		core.Input(sgIn{Topic: "x"}),
		core.ProcessOptions{},
	)
	select {
	case <-blockingExited:
	default:
		t.Fatal("Run returned before the canceled branch exited")
	}
	if proc.Status() == core.StatusCompleted {
		t.Fatal("expected failed process after generator error")
	}
}

// TestScatterGather_GeneratorsGetIsolatedContext pins the workflow concurrency
// model: every generator gets private scratch and a Blackboard fork; branch
// writes/conditions are discarded, and lifecycle/managed interaction are
// rejected because one parent Process cannot own competing continuations.
func TestScatterGather_GeneratorsGetIsolatedContext(t *testing.T) {
	probe := func(_ context.Context, pc *core.ProcessContext, _ sgIn) (sgElement, error) {
		pc.Blackboard().Store("branch-write", sgElement{Score: 99})
		pc.Blackboard().StoreCondition("branch-condition", true)
		if status, err := pc.Suspend(context.Background(), agent.Suspension{}); status != core.ActionFailed || !errors.Is(err, core.ErrParallelBranchControl) {
			return sgElement{}, errors.New("parallel suspension was not rejected")
		}
		if err := pc.TerminateAgent("must stay branch-local"); !errors.Is(err, core.ErrParallelBranchControl) {
			return sgElement{}, err
		}
		if _, err := pc.Interact(context.Background(), core.Interaction{}); !errors.Is(err, core.ErrParallelBranchControl) {
			return sgElement{}, err
		}
		return sgElement{Score: 1}, nil
	}
	gens := make([]func(context.Context, *core.ProcessContext, sgIn) (sgElement, error), 8)
	for i := range gens {
		gens[i] = probe
	}

	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name:       "isolated",
		Generators: gens,
		Joiner: func(_ context.Context, _ *core.ProcessContext, items []sgElement) (sgResult, error) {
			return sgResult{Total: len(items)}, nil
		},
	})
	if err != nil {
		t.Fatalf("ScatterGather: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, err := engine.Run(t.Context(), a,
		core.Input(sgIn{Topic: "x"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	if _, ok := proc.Blackboard().Load("branch-write"); ok {
		t.Fatal("parallel branch named write leaked into parent blackboard")
	}
	if _, ok := proc.Blackboard().Condition("branch-condition"); ok {
		t.Fatal("parallel branch condition leaked into parent blackboard")
	}
}

func TestScatterGather_GeneratorErrorPropagates(t *testing.T) {
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name: "fanout-err",
		Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
			func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
				return sgElement{Score: 1}, nil
			},
			func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
				return sgElement{}, errors.New("boom")
			},
		},
		Joiner: func(_ context.Context, _ *core.ProcessContext, items []sgElement) (sgResult, error) {
			return sgResult{Total: len(items)}, nil
		},
	})
	if err != nil {
		t.Fatalf("ScatterGather: %v", err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, _ := engine.Run(t.Context(), a,
		core.Input(sgIn{Topic: "x"}),
		core.ProcessOptions{},
	)
	if proc.Status() == core.StatusCompleted {
		t.Fatal("expected non-completed status when a generator errors")
	}
	if proc.Failure() == nil || !strings.Contains(proc.Failure().Error(), "boom") {
		t.Fatalf("expected failure containing 'boom', got %v", proc.Failure())
	}
}

func TestScatterGather_CancelledQueuedGeneratorsDoNotStart(t *testing.T) {
	cause := errors.New("first generator failed")
	var laterCalls atomic.Int32
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name:           "fanout-cancel-queue",
		MaxConcurrency: 1,
		Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
			func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
				return sgElement{}, cause
			},
			func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
				laterCalls.Add(1)
				return sgElement{}, nil
			},
			func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
				laterCalls.Add(1)
				return sgElement{}, nil
			},
		},
		Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) {
			return sgResult{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	process, err := engine.Run(t.Context(), a, core.Input(sgIn{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusFailed || !errors.Is(process.Failure(), cause) {
		t.Fatalf("status/failure = %s/%v, want first generator failure", process.Status(), process.Failure())
	}
	if calls := laterCalls.Load(); calls != 0 {
		t.Fatalf("queued generator calls = %d, want none after cancellation", calls)
	}
}

func TestScatterGather_OwnsGeneratorSlice(t *testing.T) {
	originalCalls := 0
	replacementCalls := 0
	generators := []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
		func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
			originalCalls++
			return sgElement{Score: 1}, nil
		},
	}
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name:       "fanout-owned-config",
		Generators: generators,
		Joiner: func(_ context.Context, _ *core.ProcessContext, items []sgElement) (sgResult, error) {
			return sgResult{Total: items[0].Score}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	generators[0] = func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
		replacementCalls++
		return sgElement{Score: 99}, nil
	}

	engine := agent.MustNewEngine(runtime.Config{})
	process, err := engine.Run(t.Context(), a, core.Input(sgIn{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	result, ok := core.Result[sgResult](process)
	if !ok || result.Total != 1 || originalCalls != 1 || replacementCalls != 0 {
		t.Fatalf("result/original/replacement = %#v/%d/%d, want score 1 from owned generator", result, originalCalls, replacementCalls)
	}
}

func TestScatterGather_RejectsInvalidSpec(t *testing.T) {
	cases := []struct {
		name string
		spec workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]
	}{
		{"empty name", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
				func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) { return sgElement{}, nil },
			},
			Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
		}},
		{"empty generators", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Name:   "x",
			Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
		}},
		{"nil joiner", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Name: "x",
			Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
				func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) { return sgElement{}, nil },
			},
		}},
		{"nil generator", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Name: "x",
			Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
				nil,
			},
			Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := workflow.ScatterGather(tc.spec); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
