package core

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/tool"
)

func TestProcessContextToolMethodsNormalizeNilContext(t *testing.T) {
	var calls int
	pc := &ProcessContext{
		actionTools: func(ctx context.Context, roles []string) ([]tool.Tool, error) {
			if ctx == nil {
				t.Fatal("resolver received nil context")
			}
			calls++
			return nil, nil
		},
		actionToolGroups: []string{"action-tools"},
	}

	//lint:ignore SA1012 nil normalization is the contract under test
	if _, err := pc.ActionTools(nil); err != nil { //nolint:staticcheck // Keep golangci-lint aligned with staticcheck.
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

	//lint:ignore SA1012 nil normalization is the contract under test
	ctx, cancel := pc.ToolCallContext(nil) //nolint:staticcheck // Keep golangci-lint aligned with staticcheck.
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

func TestNilProcessContextToolHelpersUseEmptyCapabilities(t *testing.T) {
	var pc *ProcessContext
	if tools, err := pc.ActionTools(t.Context()); err != nil || tools != nil {
		t.Fatalf("ActionTools = %v, %v; want nil, nil", tools, err)
	}
	ctx, cancel := pc.ToolCallContext(t.Context())
	cancel()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("nil ProcessContext cancel did not cancel derived context")
	}
}
