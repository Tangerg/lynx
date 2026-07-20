package workflow_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/storetest"
	"github.com/Tangerg/lynx/agent/workflow"
)

type ruIn struct{ Target int }
type ruOut struct{ Value int }

// refine is used as BOTH In and Out to exercise the In==Out refinement-loop
// shape, where each iteration's output would otherwise shadow the original
// input on the blackboard.
type refine struct {
	Tag string
	N   int
}

// TestRepeatUntil_InEqualsOut guards the In==Out shadowing bug: when the loop's
// input and output are the same Go type (the canonical "refine a draft until
// good enough"), the per-iteration outputs must NOT shadow the original input —
// both Task and Accept must keep seeing the ORIGINAL input, not the latest
// attempt. With the bug, iteration 2+ saw the previous attempt as `in`.
func TestRepeatUntil_InEqualsOut(t *testing.T) {
	var taskInputs, acceptInputs []string
	a, err := workflow.RepeatUntil(workflow.RepeatUntilConfig[refine, refine]{
		Name:          "refine-loop",
		MaxIterations: 5,
		Task: func(_ context.Context, _ *core.ProcessContext, in refine, h *workflow.History[refine]) (refine, error) {
			taskInputs = append(taskInputs, in.Tag)
			return refine{Tag: "attempt", N: h.Count() + 1}, nil
		},
		Accept: func(_ context.Context, in refine, _ refine, h *workflow.History[refine]) bool {
			acceptInputs = append(acceptInputs, in.Tag)
			return h.Count() >= 3 // stop after 3 attempts
		},
	})
	if err != nil {
		t.Fatalf("RepeatUntil: %v", err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, err := engine.Run(t.Context(), a,
		core.Input(refine{Tag: "orig"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	if len(taskInputs) < 2 {
		t.Fatalf("expected ≥2 task iterations to exercise shadowing, got %d", len(taskInputs))
	}
	for i, tag := range taskInputs {
		if tag != "orig" {
			t.Errorf("Task iteration %d saw in.Tag=%q, want \"orig\" (output shadowed the In==Out input)", i, tag)
		}
	}
	for i, tag := range acceptInputs {
		if tag != "orig" {
			t.Errorf("Accept call %d saw in.Tag=%q, want \"orig\" (In==Out shadowing)", i, tag)
		}
	}
}

func TestRepeatUntil_LoopsUntilAccept(t *testing.T) {
	// Task increments by 1 each call. Accept stops once value ≥ Target.
	a, err := workflow.RepeatUntil(workflow.RepeatUntilConfig[ruIn, ruOut]{
		Name:          "increment-loop",
		Description:   "increments until target",
		MaxIterations: 10,
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruIn, h *workflow.History[ruOut]) (ruOut, error) {
			last, ok := h.Last()
			if !ok {
				return ruOut{Value: 1}, nil
			}
			return ruOut{Value: last.Value + 1}, nil
		},
		Accept: func(_ context.Context, in ruIn, last ruOut, _ *workflow.History[ruOut]) bool {
			return last.Value >= in.Target
		},
	})
	if err != nil {
		t.Fatalf("RepeatUntil: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{})
	_, err = engine.Deploy(a)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	var proc *runtime.Process
	proc, err = engine.Run(t.Context(), a,
		core.Input(ruIn{Target: 4}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.Result[ruOut](proc)
	if !ok {
		t.Fatal("no ruOut bound")
	}
	if got.Value != 4 {
		t.Fatalf("Value = %d, want 4", got.Value)
	}
}

func TestRepeatUntil_MaxIterationsCap(t *testing.T) {
	// Accept never returns true; MaxIterations forces termination at 3.
	a, err := workflow.RepeatUntil(workflow.RepeatUntilConfig[ruIn, ruOut]{
		Name:          "capped-loop",
		MaxIterations: 3,
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruIn, h *workflow.History[ruOut]) (ruOut, error) {
			return ruOut{Value: h.Count() + 1}, nil
		},
		Accept: func(context.Context, ruIn, ruOut, *workflow.History[ruOut]) bool { return false },
	})
	if err != nil {
		t.Fatalf("RepeatUntil: %v", err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	_, err = engine.Deploy(a)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, _ := engine.Run(t.Context(), a,
		core.Input(ruIn{Target: 999}),
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, _ := core.Result[ruOut](proc)
	if got.Value != 3 {
		t.Fatalf("Value = %d, want 3 (MaxIterations cap)", got.Value)
	}
}

func TestRepeatUntil_AutoSnapshotRoundTrip(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{
		BuildID: "repeat-until-snapshot", ProcessStore: store, AutoSnapshot: true,
	})
	a, err := workflow.RepeatUntil(workflow.RepeatUntilConfig[ruIn, ruOut]{
		Name: "durable-repeat", MaxIterations: 3,
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruIn, history *workflow.History[ruOut]) (ruOut, error) {
			return ruOut{Value: history.Count() + 1}, nil
		},
		Accept: func(context.Context, ruIn, ruOut, *workflow.History[ruOut]) bool { return false },
	})
	if err != nil {
		t.Fatal(err)
	}
	mustDeploy(t, engine, a)
	process, err := engine.Run(t.Context(), a, core.Input(ruIn{Target: 9}), core.ProcessOptions{})
	if err != nil || process.Status() != core.StatusCompleted {
		t.Fatalf("Run status=%s err=%v failure=%v", process.Status(), err, process.Failure())
	}
	restored, err := engine.Restore(t.Context(), process.ID(), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	result, ok := core.Result[ruOut](restored)
	if !ok || result.Value != 3 {
		t.Fatalf("restored result=%#v ok=%v, want value 3", result, ok)
	}
	history, ok := core.Result[*workflow.History[ruOut]](restored)
	if !ok || history.Count() != 3 {
		t.Fatalf("restored history count=%d ok=%v, want 3", history.Count(), ok)
	}
}

func TestRepeatUntil_HistoryPassedToTaskAndAccept(t *testing.T) {
	var seenInTask []int
	a, err := workflow.RepeatUntil(workflow.RepeatUntilConfig[ruIn, ruOut]{
		Name:          "history-witness",
		MaxIterations: 5,
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruIn, h *workflow.History[ruOut]) (ruOut, error) {
			snapshot := make([]int, 0, h.Count())
			for _, a := range h.Attempts() {
				snapshot = append(snapshot, a.Value)
			}
			seenInTask = snapshot // overwrite each iteration
			return ruOut{Value: h.Count() + 1}, nil
		},
		Accept: func(_ context.Context, _ ruIn, last ruOut, h *workflow.History[ruOut]) bool {
			return last.Value >= 3 && h.Count() >= 3
		},
	})
	if err != nil {
		t.Fatalf("RepeatUntil: %v", err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	_, err = engine.Deploy(a)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, _ := engine.Run(t.Context(), a,
		core.Input(ruIn{Target: 0}),
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	// At the start of iteration 3, task should have seen [1, 2].
	want := []int{1, 2}
	if len(seenInTask) != len(want) {
		t.Fatalf("seenInTask = %v, want %v", seenInTask, want)
	}
	for i := range want {
		if seenInTask[i] != want[i] {
			t.Fatalf("seenInTask[%d] = %d, want %d", i, seenInTask[i], want[i])
		}
	}
}

func TestRepeatUntil_RejectsInvalidSpec(t *testing.T) {
	cases := []struct {
		name string
		spec workflow.RepeatUntilConfig[ruIn, ruOut]
	}{
		{"empty name", workflow.RepeatUntilConfig[ruIn, ruOut]{
			Task: func(context.Context, *core.ProcessContext, ruIn, *workflow.History[ruOut]) (ruOut, error) {
				return ruOut{}, nil
			},
			Accept: func(context.Context, ruIn, ruOut, *workflow.History[ruOut]) bool { return true },
		}},
		{"nil task", workflow.RepeatUntilConfig[ruIn, ruOut]{
			Name:   "x",
			Accept: func(context.Context, ruIn, ruOut, *workflow.History[ruOut]) bool { return true },
		}},
		{"nil accept", workflow.RepeatUntilConfig[ruIn, ruOut]{
			Name: "x",
			Task: func(context.Context, *core.ProcessContext, ruIn, *workflow.History[ruOut]) (ruOut, error) {
				return ruOut{}, nil
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := workflow.RepeatUntil(tc.spec); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
