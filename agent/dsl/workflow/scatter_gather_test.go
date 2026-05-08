package workflow_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/dsl/workflow"
	"github.com/Tangerg/lynx/agent/runtime"
)

type sgIn struct{ Topic string }
type sgElement struct{ Score int }
type sgResult struct{ Total int }

func TestScatterGather_RunsAllGeneratorsAndJoins(t *testing.T) {
	var inFlightPeak int32
	gen := func(score int) func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) {
		return func(ctx context.Context, _ *core.ProcessContext, _ sgIn) (sgElement, error) {
			now := atomic.AddInt32(&inFlightPeak, 1)
			defer atomic.AddInt32(&inFlightPeak, -1)
			// keep a couple goroutines overlapping so the peak shows
			time.Sleep(10 * time.Millisecond)
			_ = now
			return sgElement{Score: score}, nil
		}
	}

	a := workflow.ScatterGatherAgent(workflow.ScatterGatherSpec[sgIn, sgElement, sgResult]{
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

	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, err := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: sgIn{Topic: "test"}},
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("status = %s; failure = %v", proc.Status(), proc.Failure())
	}
	got, ok := core.ResultOfType[sgResult](proc)
	if !ok {
		t.Fatal("no sgResult bound")
	}
	if got.Total != 1+2+3 {
		t.Fatalf("Total = %d, want 6", got.Total)
	}
}

func TestScatterGather_GeneratorErrorPropagates(t *testing.T) {
	a := workflow.ScatterGatherAgent(workflow.ScatterGatherSpec[sgIn, sgElement, sgResult]{
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
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	if err := platform.Deploy(a); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	proc, _ := platform.RunAgent(context.Background(), a,
		map[string]any{core.DefaultBindingName: sgIn{Topic: "x"}},
		core.ProcessOptions{},
	)
	if proc.Status() == core.StatusCompleted {
		t.Fatal("expected non-completed status when a generator errors")
	}
	if proc.Failure() == nil || !strings.Contains(proc.Failure().Error(), "boom") {
		t.Fatalf("expected failure containing 'boom', got %v", proc.Failure())
	}
}

func TestScatterGather_PanicsOnInvalidSpec(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
	}{
		{"empty name", func() {
			workflow.ScatterGatherAgent(workflow.ScatterGatherSpec[sgIn, sgElement, sgResult]{
				Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
					func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) { return sgElement{}, nil },
				},
				Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
			})
		}},
		{"empty generators", func() {
			workflow.ScatterGatherAgent(workflow.ScatterGatherSpec[sgIn, sgElement, sgResult]{
				Name:   "x",
				Joiner: func(context.Context, *core.ProcessContext, []sgElement) (sgResult, error) { return sgResult{}, nil },
			})
		}},
		{"nil joiner", func() {
			workflow.ScatterGatherAgent(workflow.ScatterGatherSpec[sgIn, sgElement, sgResult]{
				Name: "x",
				Generators: []func(context.Context, *core.ProcessContext, sgIn) (sgElement, error){
					func(context.Context, *core.ProcessContext, sgIn) (sgElement, error) { return sgElement{}, nil },
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
