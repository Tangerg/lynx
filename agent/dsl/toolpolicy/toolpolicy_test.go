package toolpolicy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/dsl/toolpolicy"
	"github.com/Tangerg/lynx/core/model/chat"
)

type fakeTool struct {
	name    string
	calls   int
	gotArgs []string
	resp    string
	respErr error
}

func (t *fakeTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: t.name, InputSchema: `{"type":"object"}`}
}
func (t *fakeTool) Metadata() chat.ToolMetadata { return chat.ToolMetadata{} }
func (t *fakeTool) Call(_ context.Context, args string) (string, error) {
	t.calls++
	t.gotArgs = append(t.gotArgs, args)
	return t.resp, t.respErr
}

// ---------- WithOnceOnly --------------------------------------------------

func TestWithOnceOnly_FirstCallSucceedsSecondRejects_LoopScope(t *testing.T) {
	inner := &fakeTool{name: "search", resp: "ok"}
	wrapped := toolpolicy.WithOnceOnly(inner)

	ctx := toolpolicy.WithLoopScope(context.Background())
	if _, err := wrapped.Call(ctx, `{"q":"a"}`); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("delegate calls = %d, want 1", inner.calls)
	}

	_, err := wrapped.Call(ctx, `{"q":"b"}`)
	if err == nil {
		t.Fatal("expected ErrToolAlreadyCalled on second call")
	}
	if !errors.Is(err, toolpolicy.ErrToolAlreadyCalled) {
		t.Fatalf("error = %v, want ErrToolAlreadyCalled", err)
	}
	if inner.calls != 1 {
		t.Fatalf("delegate must NOT run twice; calls = %d", inner.calls)
	}
}

func TestWithOnceOnly_DifferentScopesAreIndependent(t *testing.T) {
	inner := &fakeTool{name: "search", resp: "ok"}
	wrapped := toolpolicy.WithOnceOnly(inner)

	ctx1 := toolpolicy.WithLoopScope(context.Background())
	ctx2 := toolpolicy.WithLoopScope(context.Background())

	if _, err := wrapped.Call(ctx1, `{}`); err != nil {
		t.Fatalf("ctx1 call: %v", err)
	}
	// Same tool fires fine in a fresh scope.
	if _, err := wrapped.Call(ctx2, `{}`); err != nil {
		t.Fatalf("ctx2 call (different scope) should succeed: %v", err)
	}
	if inner.calls != 2 {
		t.Fatalf("delegate calls = %d, want 2 (one per scope)", inner.calls)
	}
}

func TestWithOnceOnly_FallbackToProcessWideWithoutScope(t *testing.T) {
	inner := &fakeTool{name: "x", resp: "ok"}
	wrapped := toolpolicy.WithOnceOnly(inner)

	if _, err := wrapped.Call(context.Background(), `{}`); err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, err := wrapped.Call(context.Background(), `{}`)
	if !errors.Is(err, toolpolicy.ErrToolAlreadyCalled) {
		t.Fatalf("expected ErrToolAlreadyCalled with no scope, got %v", err)
	}
}

func TestWithOnceOnly_PanicsOnNilTool(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	toolpolicy.WithOnceOnly(nil)
}

// ---------- WithUnlock ----------------------------------------------------

func TestWithUnlock_LockedReturnsErr(t *testing.T) {
	inner := &fakeTool{name: "delete", resp: "deleted"}
	wrapped := toolpolicy.WithUnlock(inner, func(context.Context, string) (bool, string) {
		return false, "needs admin auth"
	})

	_, err := wrapped.Call(context.Background(), `{"id":1}`)
	if err == nil {
		t.Fatal("expected error when locked")
	}
	if !errors.Is(err, toolpolicy.ErrToolLocked) {
		t.Fatalf("expected ErrToolLocked, got %v", err)
	}
	if inner.calls != 0 {
		t.Fatalf("delegate must not run when locked; calls = %d", inner.calls)
	}
}

func TestWithUnlock_UnlockedDelegatesNormally(t *testing.T) {
	inner := &fakeTool{name: "delete", resp: "deleted"}
	wrapped := toolpolicy.WithUnlock(inner, func(context.Context, string) (bool, string) {
		return true, ""
	})

	out, err := wrapped.Call(context.Background(), `{"id":1}`)
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if out != "deleted" {
		t.Fatalf("got %q, want delete result", out)
	}
	if inner.calls != 1 {
		t.Fatalf("delegate calls = %d, want 1", inner.calls)
	}
}

func TestWithUnlock_ConditionSeesArguments(t *testing.T) {
	var seen string
	inner := &fakeTool{name: "x", resp: "ok"}
	wrapped := toolpolicy.WithUnlock(inner, func(_ context.Context, args string) (bool, string) {
		seen = args
		return true, ""
	})
	wrapped.Call(context.Background(), `{"hello":"world"}`)
	if seen != `{"hello":"world"}` {
		t.Fatalf("condition did not receive args; got %q", seen)
	}
}

func TestWithUnlock_PanicsOnNilArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func()
	}{
		{"nil tool", func() {
			toolpolicy.WithUnlock(nil, func(context.Context, string) (bool, string) { return true, "" })
		}},
		{"nil condition", func() {
			toolpolicy.WithUnlock(&fakeTool{name: "x"}, nil)
		}},
	} {
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

// ---------- Composition ---------------------------------------------------

func TestCompose_OnceOnly_AndUnlock(t *testing.T) {
	inner := &fakeTool{name: "search", resp: "ok"}
	gated := toolpolicy.WithUnlock(
		toolpolicy.WithOnceOnly(inner),
		func(context.Context, string) (bool, string) { return true, "" },
	)

	ctx := toolpolicy.WithLoopScope(context.Background())
	if _, err := gated.Call(ctx, `{}`); err != nil {
		t.Fatalf("first composed call: %v", err)
	}
	if _, err := gated.Call(ctx, `{}`); !errors.Is(err, toolpolicy.ErrToolAlreadyCalled) {
		t.Fatalf("expected ErrToolAlreadyCalled on duplicate, got %v", err)
	}
}
