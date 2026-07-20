package workflow_test

import (
	"context"
	"math"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
	"github.com/Tangerg/lynx/agent/storetest"
	"github.com/Tangerg/lynx/agent/workflow"
)

type ruaIn struct{ Topic string }
type ruaOut struct{ Draft string }

func TestRepeatUntilAcceptable_StopsWhenScoreCrossesThreshold(t *testing.T) {
	var iterations atomic.Int32

	a, err := workflow.RepeatUntilAcceptable(workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
		Name:            "iterate-draft",
		MaxIterations:   5,
		AcceptableScore: 0.7,
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruaIn, h *workflow.History[ruaOut]) (ruaOut, error) {
			n := iterations.Add(1)
			return ruaOut{Draft: "v" + string(rune('0'+n))}, nil
		},
		Evaluator: func(_ context.Context, _ *core.ProcessContext, _ ruaIn, last ruaOut) (workflow.Feedback, error) {
			// First two attempts score low (0.4), third crosses 0.8.
			score := 0.4
			if last.Draft == "v3" {
				score = 0.8
			}
			return workflow.Feedback{Score: score, Text: "feedback for " + last.Draft}, nil
		},
	})
	if err != nil {
		t.Fatalf("RepeatUntilAcceptable: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{})
	_, err = engine.Deploy(a)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, err := engine.Run(t.Context(), a,
		core.Input(ruaIn{Topic: "test"}),
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.Result[ruaOut](proc)
	if !ok {
		t.Fatal("no ruaOut bound")
	}
	if got.Draft != "v3" {
		t.Fatalf("Draft = %q, want v3", got.Draft)
	}

	// Latest Feedback should also be on the blackboard for inspection.
	if fb, ok := core.Result[workflow.Feedback](proc); !ok {
		t.Fatal("Feedback should be bound on blackboard")
	} else if fb.Score < 0.8 {
		t.Fatalf("Feedback.Score = %f, want >= 0.8", fb.Score)
	}
}

func TestRepeatUntilAcceptable_DefaultsThresholdToZeroPointSeven(t *testing.T) {
	a, err := workflow.RepeatUntilAcceptable(workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
		Name:          "iterate",
		MaxIterations: 3,
		// AcceptableScore zero → should default to 0.7
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruaIn, h *workflow.History[ruaOut]) (ruaOut, error) {
			return ruaOut{Draft: "v"}, nil
		},
		Evaluator: func(_ context.Context, _ *core.ProcessContext, _ ruaIn, _ ruaOut) (workflow.Feedback, error) {
			return workflow.Feedback{Score: 0.69, Text: "borderline"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RepeatUntilAcceptable: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	proc, _ := engine.Run(t.Context(), a,
		core.Input(ruaIn{Topic: "x"}),
		core.ProcessOptions{})
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	// 0.69 < 0.7 → never accepted; loop hits MaxIterations and gives up.
	got, _ := core.Result[ruaOut](proc)
	if got.Draft == "" {
		t.Fatal("expected the last attempted Out, got empty")
	}
}

func TestRepeatUntilAcceptable_AutoSnapshotPreservesState(t *testing.T) {
	store := storetest.NewMemoryProcessStore()
	engine := agent.MustNewEngine(runtime.Config{
		BuildID: "acceptable-snapshot", ProcessStore: store, AutoSnapshot: true,
	})
	a, err := workflow.RepeatUntilAcceptable(workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
		Name: "durable-acceptable", MaxIterations: 2, AcceptableScore: 0.9,
		Task: func(_ context.Context, _ *core.ProcessContext, input ruaIn, history *workflow.History[ruaOut]) (ruaOut, error) {
			return ruaOut{Draft: input.Topic + string(rune('1'+history.Count()))}, nil
		},
		Evaluator: func(context.Context, *core.ProcessContext, ruaIn, ruaOut) (workflow.Feedback, error) {
			return workflow.Feedback{Score: 0.5, Text: "revise"}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mustDeploy(t, engine, a)
	process, err := engine.Run(t.Context(), a, core.Input(ruaIn{Topic: "original-"}), core.ProcessOptions{})
	if err != nil || process.Status() != core.StatusCompleted {
		t.Fatalf("Run status=%s err=%v failure=%v", process.Status(), err, process.Failure())
	}
	restored, err := engine.Restore(t.Context(), process.ID(), core.ProcessOptions{})
	if err != nil {
		t.Fatal(err)
	}
	history, ok := core.Result[*workflow.AttemptHistory[ruaOut]](restored)
	if !ok || history.Count() != 2 {
		t.Fatalf("restored attempt history count=%d ok=%v, want 2", history.Count(), ok)
	}
	feedback, ok := core.Result[workflow.Feedback](restored)
	if !ok || feedback.Score != 0.5 {
		t.Fatalf("restored feedback=%#v ok=%v", feedback, ok)
	}
}

func TestRepeatUntilAcceptable_InEqualsOutKeepsOriginalInput(t *testing.T) {
	var seen []string
	a, err := workflow.RepeatUntilAcceptable(workflow.RepeatUntilAcceptableConfig[refine, refine]{
		Name: "acceptable-refine", MaxIterations: 3, AcceptableScore: 1,
		Task: func(_ context.Context, _ *core.ProcessContext, input refine, history *workflow.History[refine]) (refine, error) {
			seen = append(seen, input.Tag)
			return refine{Tag: "attempt", N: history.Count() + 1}, nil
		},
		Evaluator: func(context.Context, *core.ProcessContext, refine, refine) (workflow.Feedback, error) {
			return workflow.Feedback{Score: 0}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	process, err := engine.Run(t.Context(), a, core.Input(refine{Tag: "original"}), core.ProcessOptions{})
	if err != nil || process.Status() != core.StatusCompleted {
		t.Fatalf("Run status=%s err=%v", process.Status(), err)
	}
	for index, input := range seen {
		if input != "original" {
			t.Fatalf("Task iteration %d input=%q, want original", index, input)
		}
	}
}

// TestRepeatUntilAcceptable_ReturnsBestNotLast confirms best-of-N: when no
// attempt clears the threshold, the highest-scoring attempt is returned even
// if a later attempt scored worse.
func TestRepeatUntilAcceptable_ReturnsBestNotLast(t *testing.T) {
	var iterations atomic.Int32

	a, err := workflow.RepeatUntilAcceptable(workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
		Name:            "best-of-n",
		MaxIterations:   3,
		AcceptableScore: 0.95, // unreachable → loop runs all 3, never "accepts"
		Task: func(_ context.Context, _ *core.ProcessContext, _ ruaIn, _ *workflow.History[ruaOut]) (ruaOut, error) {
			n := iterations.Add(1)
			return ruaOut{Draft: "v" + string(rune('0'+n))}, nil
		},
		Evaluator: func(_ context.Context, _ *core.ProcessContext, _ ruaIn, last ruaOut) (workflow.Feedback, error) {
			// v1=0.5, v2=0.9 (best), v3=0.3 (worse, comes last).
			score := map[string]float64{"v1": 0.5, "v2": 0.9, "v3": 0.3}[last.Draft]
			return workflow.Feedback{Score: score, Text: last.Draft}, nil
		},
	})
	if err != nil {
		t.Fatalf("RepeatUntilAcceptable: %v", err)
	}

	engine := agent.MustNewEngine(runtime.Config{})
	mustDeploy(t, engine, a)
	proc, err := engine.Run(t.Context(), a,
		core.Input(ruaIn{Topic: "t"}),
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}

	got, ok := core.Result[ruaOut](proc)
	if !ok {
		t.Fatal("no ruaOut bound")
	}
	if got.Draft != "v2" {
		t.Fatalf("Draft = %q, want v2 (highest score 0.9), not the last attempt v3", got.Draft)
	}

	// The full attempt+feedback record is available on the blackboard.
	hist, ok := core.Result[*workflow.AttemptHistory[ruaOut]](proc)
	if !ok {
		t.Fatal("AttemptHistory should be bound on blackboard")
	}
	if hist.Count() != 3 {
		t.Fatalf("AttemptHistory.Count = %d, want 3", hist.Count())
	}
}

func TestRepeatUntilAcceptable_RejectsInvalidSpec(t *testing.T) {
	cases := []struct {
		name string
		spec workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]
	}{
		{"empty name", workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
			Task: func(context.Context, *core.ProcessContext, ruaIn, *workflow.History[ruaOut]) (ruaOut, error) {
				return ruaOut{}, nil
			},
			Evaluator: func(context.Context, *core.ProcessContext, ruaIn, ruaOut) (workflow.Feedback, error) {
				return workflow.Feedback{}, nil
			},
		}},
		{"nil task", workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
			Name: "x",
			Evaluator: func(context.Context, *core.ProcessContext, ruaIn, ruaOut) (workflow.Feedback, error) {
				return workflow.Feedback{}, nil
			},
		}},
		{"nil evaluator", workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
			Name: "x",
			Task: func(context.Context, *core.ProcessContext, ruaIn, *workflow.History[ruaOut]) (ruaOut, error) {
				return ruaOut{}, nil
			},
		}},
		{"invalid threshold", workflow.RepeatUntilAcceptableConfig[ruaIn, ruaOut]{
			Name: "x", AcceptableScore: math.NaN(),
			Task: func(context.Context, *core.ProcessContext, ruaIn, *workflow.History[ruaOut]) (ruaOut, error) {
				return ruaOut{}, nil
			},
			Evaluator: func(context.Context, *core.ProcessContext, ruaIn, ruaOut) (workflow.Feedback, error) {
				return workflow.Feedback{}, nil
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := workflow.RepeatUntilAcceptable(tc.spec); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
