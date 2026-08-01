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
	gen := func(score int) workflow.Generator[sgIn, sgElement] {
		return func(ctx context.Context, _ sgIn) (sgElement, error) {
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
		Generators: []workflow.Generator[sgIn, sgElement]{
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
	_, err = engine.Deploy(t.Context(), a)
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

func TestScatterGatherMasksParentProcessView(t *testing.T) {
	type contextKey struct{}
	const ambient = "caller value"

	var calls atomic.Int32
	generator := func(ctx context.Context, _ sgIn) (sgElement, error) {
		if process := core.ProcessViewFrom(ctx); process != nil {
			return sgElement{}, errors.New("generator received the parent process view")
		}
		if got := ctx.Value(contextKey{}); got != ambient {
			return sgElement{}, errors.New("generator lost an ordinary caller context value")
		}
		calls.Add(1)
		return sgElement{Score: 1}, nil
	}

	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name:       "fanout-capability-isolation",
		Generators: []workflow.Generator[sgIn, sgElement]{generator, generator},
		Joiner: func(_ context.Context, _ *core.ProcessContext, items []sgElement) (sgResult, error) {
			return sgResult{Total: len(items)}, nil
		},
	})
	if err != nil {
		t.Fatalf("ScatterGather: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{})
	ctx := context.WithValue(t.Context(), contextKey{}, ambient)
	process, err := engine.Run(ctx, a, core.Input(sgIn{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process.Status() != core.StatusCompleted {
		t.Fatalf("status/failure = %s/%v, want completed", process.Status(), process.Failure())
	}
	if calls.Load() != 2 {
		t.Fatalf("generator calls = %d, want 2", calls.Load())
	}
}

func TestScatterGather_GeneratorErrorCancelsSiblingsAndWaitsForExit(t *testing.T) {
	cause := errors.New("boom")
	blockingStarted := make(chan struct{})
	cancellationObserved := make(chan struct{})
	release := make(chan struct{})
	blockingExited := make(chan struct{})
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name: "fanout-cancel",
		Generators: []workflow.Generator[sgIn, sgElement]{
			func(ctx context.Context, _ sgIn) (sgElement, error) {
				close(blockingStarted)
				<-ctx.Done()
				close(cancellationObserved)
				<-release
				close(blockingExited)
				return sgElement{}, ctx.Err()
			},
			func(context.Context, sgIn) (sgElement, error) {
				<-blockingStarted
				return sgElement{}, cause
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
	if _, err := engine.Deploy(t.Context(), a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	type runResult struct {
		process *runtime.Process
		err     error
	}
	done := make(chan runResult, 1)
	go func() {
		process, runErr := engine.Run(t.Context(), a,
			core.Input(sgIn{Topic: "x"}),
			core.ProcessOptions{},
		)
		done <- runResult{process: process, err: runErr}
	}()
	<-cancellationObserved
	select {
	case result := <-done:
		close(release)
		t.Fatalf("Run returned before the canceled generator exited: %v", result.err)
	default:
	}
	close(release)
	result := <-done
	if result.err != nil {
		t.Fatalf("Run: %v", result.err)
	}
	select {
	case <-blockingExited:
	default:
		t.Fatal("Run returned before the canceled branch exited")
	}
	if result.process.Status() != core.StatusFailed || !errors.Is(result.process.Failure(), cause) {
		t.Fatalf("status/failure = %s/%v, want generator failure", result.process.Status(), result.process.Failure())
	}
}

func TestScatterGather_MultipleFailuresChooseLowestGeneratorIndex(t *testing.T) {
	lowIndexFailure := errors.New("generator zero failed")
	highIndexFailure := errors.New("generator one failed first")
	lowIndexStarted := make(chan struct{})
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name: "fanout-deterministic-error",
		Generators: []workflow.Generator[sgIn, sgElement]{
			func(ctx context.Context, _ sgIn) (sgElement, error) {
				close(lowIndexStarted)
				<-ctx.Done()
				return sgElement{}, lowIndexFailure
			},
			func(context.Context, sgIn) (sgElement, error) {
				<-lowIndexStarted
				return sgElement{}, highIndexFailure
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
	if process.Status() != core.StatusFailed || !errors.Is(process.Failure(), lowIndexFailure) {
		t.Fatalf("status/failure = %s/%v, want lowest-index failure", process.Status(), process.Failure())
	}
	if errors.Is(process.Failure(), highIndexFailure) {
		t.Fatalf("failure = %v, completion-order failure leaked", process.Failure())
	}
}

func TestScatterGather_GeneratorPanicBecomesProcessFailure(t *testing.T) {
	cause := errors.New("generator panic")
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name: "fanout-panic",
		Generators: []workflow.Generator[sgIn, sgElement]{
			func(context.Context, sgIn) (sgElement, error) {
				panic(cause)
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
		t.Fatalf("status/failure = %s/%v, want recovered panic", process.Status(), process.Failure())
	}
	if failure := process.Failure().Error(); !strings.Contains(failure, "generator 0 panicked") {
		t.Fatalf("failure = %q, want generator index and panic attribution", failure)
	}
}

func TestScatterGather_GeneratorErrorPropagates(t *testing.T) {
	a, err := workflow.ScatterGather(workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
		Name: "fanout-err",
		Generators: []workflow.Generator[sgIn, sgElement]{
			func(context.Context, sgIn) (sgElement, error) {
				return sgElement{Score: 1}, nil
			},
			func(context.Context, sgIn) (sgElement, error) {
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
	if _, err := engine.Deploy(t.Context(), a); err != nil {
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
		Generators: []workflow.Generator[sgIn, sgElement]{
			func(context.Context, sgIn) (sgElement, error) {
				return sgElement{}, cause
			},
			func(context.Context, sgIn) (sgElement, error) {
				laterCalls.Add(1)
				return sgElement{}, nil
			},
			func(context.Context, sgIn) (sgElement, error) {
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
	generators := []workflow.Generator[sgIn, sgElement]{
		func(context.Context, sgIn) (sgElement, error) {
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
	generators[0] = func(context.Context, sgIn) (sgElement, error) {
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
			Generators: []workflow.Generator[sgIn, sgElement]{
				func(context.Context, sgIn) (sgElement, error) { return sgElement{}, nil },
			},
			Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
		}},
		{"empty generators", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Name:   "x",
			Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
		}},
		{"negative max concurrency", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Name:           "x",
			MaxConcurrency: -1,
			Generators: []workflow.Generator[sgIn, sgElement]{
				func(context.Context, sgIn) (sgElement, error) { return sgElement{}, nil },
			},
			Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
		}},
		{"nil joiner", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Name: "x",
			Generators: []workflow.Generator[sgIn, sgElement]{
				func(context.Context, sgIn) (sgElement, error) { return sgElement{}, nil },
			},
		}},
		{"nil generator", workflow.ScatterGatherConfig[sgIn, sgElement, sgResult]{
			Name: "x",
			Generators: []workflow.Generator[sgIn, sgElement]{
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
