package runtime_test

import (
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/runtime"
)

// TestSpawnDepthCap pins the recursive-delegation backstop: chaining spawns
// (each child spawning the next) is allowed to nest — beyond depth 1, unlike
// most agents in the field — but eventually refuses with ErrMaxSpawnDepth,
// before any deeper child is created. So a runaway self-delegation fails fast
// even with no token/step budget set. (childAgent / subInput live in
// subagent_test.go, same package.)
func TestSpawnDepthCap(t *testing.T) {
	platform := agent.NewPlatform(runtime.PlatformConfig{})
	def := childAgent()
	if err := platform.Deploy(def); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	root, err := platform.RunAgent(t.Context(), def,
		map[string]any{core.DefaultBindingName: subInput{Value: 0}}, core.ProcessOptions{})
	if err != nil {
		t.Fatalf("RunAgent: %v", err)
	}

	cur := root
	depth := 0
	for {
		ctx := core.WithProcess(t.Context(), cur)
		child, err := runtime.SpawnChildProtectedOnly(ctx, platform, def, subInput{Value: depth})
		if errors.Is(err, runtime.ErrMaxSpawnDepth) {
			break // hit the backstop
		}
		if err != nil {
			t.Fatalf("spawn at depth %d: unexpected error %v", depth+1, err)
		}
		depth++
		cur = child
		if depth > 100 {
			t.Fatal("spawn depth never capped — the backstop did not trip")
		}
	}

	// Nesting beyond a single level must be allowed (the feature), and the cap
	// must have tripped (the backstop). The refused spawn created no child.
	if depth < 2 {
		t.Fatalf("cap tripped at depth %d — nesting past depth 1 should be allowed", depth)
	}
}
