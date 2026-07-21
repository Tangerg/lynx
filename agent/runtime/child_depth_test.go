package runtime_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestRunChildLimitsDepth verifies that recursive delegation fails before an
// over-depth child is registered.
func TestRunChildLimitsDepth(t *testing.T) {
	const limit = 2
	engine := agent.MustNewEngine(runtime.Config{MaxChildDepth: limit})
	var (
		deployment *runtime.Deployment
		depthErr   error
	)
	type depthInput struct{ Depth int }
	type depthOutput struct{ Depth int }
	def := agent.New(agent.AgentConfig{
		Name: "depth-limited-child",
		Actions: []agent.Action{agent.NewAction("delegate", func(ctx context.Context, _ *core.ProcessContext, input depthInput) (depthOutput, error) {
			child, err := engine.RunChild(ctx, deployment, depthInput{Depth: input.Depth + 1})
			if err != nil {
				depthErr = err
				return depthOutput{}, err
			}
			return depthOutput{Depth: input.Depth + 1}, child.TerminalError()
		}, core.ActionConfig{})},
		Goals: []*agent.Goal{agent.NewOutputGoal[depthOutput](core.GoalConfig{Description: "delegated"})},
	})
	var err error
	deployment, err = engine.Deploy(t.Context(), def)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	root, err := engine.Run(t.Context(), def, core.Input(depthInput{}), core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !errors.Is(depthErr, runtime.ErrChildDepth) {
		t.Fatalf("deepest child error = %v, want ErrChildDepth", depthErr)
	}
	if root.Status() != core.StatusFailed {
		t.Fatalf("root status = %s, want failed", root.Status())
	}
	if got := len(engine.Processes()); got != limit+1 {
		t.Fatalf("registered process count = %d, want root plus %d children", got, limit)
	}
}
