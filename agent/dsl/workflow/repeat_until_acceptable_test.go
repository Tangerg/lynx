package workflow_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/dsl/workflow"
	"github.com/Tangerg/lynx/agent/runtime"
)

type ruaIn struct{ Topic string }
type ruaOut struct{ Draft string }

func TestRepeatUntilAcceptable_StopsWhenScoreCrossesThreshold(t *testing.T) {
	var iterations atomic.Int32

	a := workflow.RepeatUntilAcceptableAgent(workflow.RepeatUntilAcceptableSpec[ruaIn, ruaOut]{
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

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, err := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: ruaIn{Topic: "test"}},
		core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[ruaOut](proc)
	if !ok {
		t.Fatal("no ruaOut bound")
	}
	if got.Draft != "v3" {
		t.Fatalf("Draft = %q, want v3", got.Draft)
	}

	// Latest Feedback should also be on the blackboard for inspection.
	if fb, ok := core.ResultOfType[workflow.Feedback](proc); !ok {
		t.Fatal("Feedback should be bound on blackboard")
	} else if fb.Score < 0.8 {
		t.Fatalf("Feedback.Score = %f, want >= 0.8", fb.Score)
	}
}

func TestRepeatUntilAcceptable_DefaultsThresholdToZeroPointSeven(t *testing.T) {
	a := workflow.RepeatUntilAcceptableAgent(workflow.RepeatUntilAcceptableSpec[ruaIn, ruaOut]{
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

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	platform.Deploy(a)
	proc, _ := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: ruaIn{Topic: "x"}},
		core.ProcessOptions{})
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	// 0.69 < 0.7 → never accepted; loop hits MaxIterations and gives up.
	got, _ := core.ResultOfType[ruaOut](proc)
	if got.Draft == "" {
		t.Fatal("expected the last attempted Out, got empty")
	}
}

func TestRepeatUntilAcceptable_PanicsOnInvalidSpec(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"empty name", func() {
			workflow.RepeatUntilAcceptableAgent(workflow.RepeatUntilAcceptableSpec[ruaIn, ruaOut]{
				Task: func(context.Context, *core.ProcessContext, ruaIn, *workflow.History[ruaOut]) (ruaOut, error) {
					return ruaOut{}, nil
				},
				Evaluator: func(context.Context, *core.ProcessContext, ruaIn, ruaOut) (workflow.Feedback, error) {
					return workflow.Feedback{}, nil
				},
			})
		}},
		{"nil task", func() {
			workflow.RepeatUntilAcceptableAgent(workflow.RepeatUntilAcceptableSpec[ruaIn, ruaOut]{
				Name: "x",
				Evaluator: func(context.Context, *core.ProcessContext, ruaIn, ruaOut) (workflow.Feedback, error) {
					return workflow.Feedback{}, nil
				},
			})
		}},
		{"nil evaluator", func() {
			workflow.RepeatUntilAcceptableAgent(workflow.RepeatUntilAcceptableSpec[ruaIn, ruaOut]{
				Name: "x",
				Task: func(context.Context, *core.ProcessContext, ruaIn, *workflow.History[ruaOut]) (ruaOut, error) {
					return ruaOut{}, nil
				},
			})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Fatal("expected panic")
				}
			}()
			tc.fn()
		})
	}
}
