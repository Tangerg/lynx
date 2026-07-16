package runtime_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestRunChildLimitsDepth verifies that recursive delegation fails before an
// over-depth child is registered.
func TestRunChildLimitsDepth(t *testing.T) {
	engine := agent.MustNewEngine(runtime.Config{})
	def := childAgent()
	deployment, err := engine.Deploy(def)
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	root, err := engine.Run(t.Context(), def,
		map[string]any{core.DefaultBindingName: subInput{Value: 0}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	parent := root
	depth := 0
	for {
		ctx := core.WithProcessView(t.Context(), parent)
		child, err := engine.RunChild(ctx, deployment, subInput{Value: depth})
		if errors.Is(err, runtime.ErrChildDepth) {
			break
		}
		if err != nil {
			t.Fatalf("run child at depth %d: %v", depth+1, err)
		}
		depth++
		parent = child
		if depth > 100 {
			t.Fatal("child depth was not limited")
		}
	}

	if depth < 2 {
		t.Fatalf("cap tripped at depth %d — nesting past depth 1 should be allowed", depth)
	}
}
