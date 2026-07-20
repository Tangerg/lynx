package core

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/tools"
)

func TestProcessContextToolMethodsNormalizeNilContext(t *testing.T) {
	var calls int
	pc := &ProcessContext{
		actionTools: func(ctx context.Context, requirements []ToolGroupRequirement) ([]tools.Tool, error) {
			if ctx == nil {
				t.Fatal("resolver received nil context")
			}
			calls++
			return nil, nil
		},
		actionToolGroups: []ToolGroupRequirement{{Role: "action-tools"}},
	}

	if _, err := pc.ActionTools(nil); err != nil { //nolint:staticcheck // Intentionally exercises nil context normalization.
		t.Fatalf("ActionTools: %v", err)
	}
	if calls != 1 {
		t.Fatalf("resolver calls = %d, want 1", calls)
	}
}

func TestProcessContextToolCallContextNormalizesNilParent(t *testing.T) {
	var released bool
	pc := &ProcessContext{
		toolCallCancel: func(cancel context.CancelFunc) func() {
			if cancel == nil {
				t.Fatal("registered nil cancel func")
			}
			return func() { released = true }
		},
	}

	ctx, cancel := pc.ToolCallContext(nil) //nolint:staticcheck // Intentionally exercises nil context normalization.
	if ctx == nil {
		t.Fatal("ToolCallContext returned nil context")
	}
	cancel()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("ToolCallContext cancel did not cancel child context")
	}
	if !released {
		t.Fatal("ToolCallContext cancel did not release runtime registration")
	}
}
