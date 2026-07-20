package runtime_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/event"
	"github.com/Tangerg/lynx/agent/runtime"
)

type word struct{ Text string }
type wordCount struct{ Count int }

// stuckCounter is an EventListener extension that counts ProcessStuck
// occurrences via the supplied pointer.
type stuckCounter struct{ count *int }

func (stuckCounter) Name() string { return "stuck-counter" }
func (s stuckCounter) OnEvent(_ context.Context, e event.Event) {
	if _, ok := e.(event.ProcessStuck); ok {
		*s.count++
	}
}

// TestRunSingleAction verifies the smallest end-to-end loop: one input, one
// action, one goal. Ensures the planner finds the (single) action and the
// runtime executes it to completion.
func TestRunSingleAction(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "test", Actions: []agent.Action{agent.NewAction("count", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "word counted"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatal(err)
	}

	proc, err := engine.Run(
		context.Background(), a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusCompleted {
		t.Fatalf("expected completed, got %s; failure=%v", proc.Status(), proc.Failure())
	}
	got, ok := core.Result[wordCount](proc)
	if !ok {
		t.Fatal("no wordCount produced")
	}
	if got.Count != 4 {
		t.Fatalf("count: got %d want 4", got.Count)
	}
}

func TestRunPreservesPanickedActionCause(t *testing.T) {
	cause := errors.New("action sentinel")
	a := agent.New(agent.AgentConfig{
		Name: "panicking-action",
		Actions: []agent.Action{agent.NewAction("panic", func(context.Context, *core.ProcessContext, word) (wordCount, error) {
			panic(cause)
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "word counted"})},
	})
	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	process, err := engine.Run(
		t.Context(),
		a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if process == nil || process.Status() != core.StatusFailed {
		t.Fatalf("process = %#v, want failed process", process)
	}
	failure := process.Failure()
	if !errors.Is(failure, cause) {
		t.Fatalf("process failure = %v, want wrapped panic cause", failure)
	}
	if !strings.Contains(failure.Error(), "runtime.Process.invokeAction: action panicked") {
		t.Fatalf("process failure = %v, want panic boundary context", failure)
	}
}

// TestRunMultiStepPlanning confirms the GOAP planner sequences three actions
// correctly: A produces X, B consumes X to produce Y, C consumes Y to produce
// the goal type.
func TestRunMultiStepPlanning(t *testing.T) {
	type stage1 struct{ V int }
	type stage2 struct{ V int }
	type stage3 struct{ V int }

	a := agent.New(agent.AgentConfig{Name: "multi", Actions: []agent.Action{agent.NewAction("a", func(ctx context.Context, pc *core.ProcessContext, in word) (stage1, error) {
		return stage1{V: len(in.Text)}, nil
	}, core.ActionConfig{}), agent.NewAction("b", func(ctx context.Context, pc *core.ProcessContext, in stage1) (stage2, error) {
		return stage2{V: in.V * 2}, nil
	}, core.ActionConfig{}), agent.NewAction("c", func(ctx context.Context, pc *core.ProcessContext, in stage2) (stage3, error) {
		return stage3{V: in.V + 1}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[stage3](core.GoalConfig{Description: "stage3 produced"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	if _, err := engine.Deploy(a); err != nil {
		t.Fatal(err)
	}

	proc, err := engine.Run(
		context.Background(), a,
		core.Input(word{Text: "abcd"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, ok := core.Result[stage3](proc)
	if !ok {
		t.Fatalf("no stage3; status=%s", proc.Status())
	}
	if got.V != 9 {
		t.Fatalf("got %d want 9", got.V)
	}
	// Three actions, three ticks.
	if len(proc.History()) != 3 {
		t.Fatalf("history length %d, want 3", len(proc.History()))
	}
}

func TestRunValidatesBeforeCreatingProcess(t *testing.T) {
	a := core.NewAgent(core.AgentConfig{
		Name:    "bad",
		Actions: []core.Action{nil},
		Goals:   []*core.Goal{core.NewGoal(core.GoalConfig{Name: "goal"})},
	})

	engine := agent.MustNewEngine(runtime.Config{})
	proc, err := engine.Run(context.Background(), a, core.Bindings{}, core.ProcessOptions{})
	if err == nil {
		t.Fatal("Run should reject invalid agent")
	}
	if proc != nil {
		t.Fatalf("process = %v, want nil", proc)
	}
	if !strings.Contains(err.Error(), "action at index 0 is nil") {
		t.Fatalf("Run error = %v, want validation detail", err)
	}
}

func TestRunRejectsUnknownPlannerName(t *testing.T) {
	a := agent.New(agent.AgentConfig{Name: "unknown-planner", Actions: []agent.Action{agent.NewAction("count", func(ctx context.Context, pc *core.ProcessContext, in word) (wordCount, error) {
		return wordCount{Count: len(in.Text)}, nil
	}, core.ActionConfig{})}, Goals: []*agent.Goal{agent.NewOutputGoal[wordCount](core.GoalConfig{Description: "word counted"})}, PlannerName: "nonexistent"})

	engine := agent.MustNewEngine(runtime.Config{})

	proc, err := engine.Run(
		context.Background(), a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err == nil {
		t.Fatal("Run should reject unknown planner name")
	}
	if proc != nil {
		t.Fatalf("process = %v, want nil", proc)
	}
	if !strings.Contains(err.Error(), `planner "nonexistent" which is not registered`) {
		t.Fatalf("Run error = %v, want unregistered-planner detail", err)
	}
}

func TestRunPublishesSingleStuckEvent(t *testing.T) {
	type unusedIn struct{}
	type unusedOut struct{}

	a := core.NewAgent(core.AgentConfig{
		Name: "stuck",
		Actions: []core.Action{
			core.NewAction("unused",
				func(ctx context.Context, pc *core.ProcessContext, in unusedIn) (unusedOut, error) {
					return unusedOut{}, nil
				},
				// The goal is statically reachable (this action produces
				// "never") but no unusedIn is bound at runtime, so planning
				// legitimately reaches the Stuck state after deployment
				// validation succeeds.
				core.ActionConfig{Effects: []string{"never"}},
			),
		},
		Goals: []*core.Goal{core.NewGoal(core.GoalConfig{Name: "never", Preconditions: []string{"never"}})},
	})

	stuckEvents := 0
	engine := agent.MustNewEngine(runtime.Config{
		Extensions: []core.Extension{
			stuckCounter{count: &stuckEvents},
		},
	})

	proc, err := engine.Run(context.Background(), a, core.Bindings{}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusStuck {
		t.Fatalf("status = %s, want stuck", proc.Status())
	}
	if stuckEvents != 1 {
		t.Fatalf("stuck events = %d, want 1", stuckEvents)
	}
}

func TestRunMarksCancelledDuringActionAsKilled(t *testing.T) {
	type out struct{}
	ctx, cancel := context.WithCancel(t.Context())
	actionErr := errors.New("transient")
	attempts := 0

	a := agent.New(agent.AgentConfig{Name: "cancel", Actions: []agent.Action{agent.NewAction("cancel", func(ctx context.Context, pc *core.ProcessContext, in word) (out, error) {
		attempts++
		cancel()
		return out{}, actionErr
	}, core.ActionConfig{Retry: core.RetryPolicy{MaxAttempts: 3, Safety: core.RetrySafetyIdempotent}})}, Goals: []*agent.Goal{agent.NewOutputGoal[out](core.GoalConfig{Description: "canceled"})}})

	engine := agent.MustNewEngine(runtime.Config{})
	proc, err := engine.Run(
		ctx, a,
		core.Input(word{Text: "lynx"}),
		core.ProcessOptions{},
	)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if proc.Status() != core.StatusKilled {
		t.Fatalf("status = %s, want killed; failure=%v", proc.Status(), proc.Failure())
	}
	if !errors.Is(proc.Failure(), context.Canceled) {
		t.Fatalf("failure = %v, want context.Canceled", proc.Failure())
	}
	if attempts != 1 {
		t.Fatalf("canceled action attempts = %d, want 1", attempts)
	}
}
