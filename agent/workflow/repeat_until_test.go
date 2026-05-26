package workflow_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/workflow"
)

type ruIn struct{ Target int }
type ruOut struct{ Value int }

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

	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, err := platform.RunAgent(t.Context(), a,
		map[string]any{core.DefaultBindingName: ruIn{Target: 4}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[ruOut](proc)
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
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, _ := platform.RunAgent(t.Context(), a,
		map[string]any{core.DefaultBindingName: ruIn{Target: 999}},
		core.ProcessOptions{},
	)
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, _ := core.ResultOfType[ruOut](proc)
	if got.Value != 3 {
		t.Fatalf("Value = %d, want 3 (MaxIterations cap)", got.Value)
	}
}

func TestRepeatUntil_HistoryPassedToTaskAndAccept(t *testing.T) {
	var seenInTask []int
	a, err := workflow.RepeatUntil(workflow.RepeatUntilConfig[ruIn, ruOut]{
		Name:          "history-witness",
		MaxIterations: 5,
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruIn, h *workflow.History[ruOut]) (ruOut, error) {
			snapshot := make([]int, 0, h.Count())
			for _, a := range h.Attempts {
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
	platform := agent.NewPlatform(&runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, _ := platform.RunAgent(t.Context(), a,
		map[string]any{core.DefaultBindingName: ruIn{Target: 0}},
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
