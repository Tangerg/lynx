package toolpolicy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/lynx/agent/toolpolicy"
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

// ---------- OnceOnly --------------------------------------------------

func TestOnceOnly_FirstCallSucceedsSecondRejects_LoopScope(t *testing.T) {
	inner := &fakeTool{name: "search", resp: "ok"}
	wrapped, _ := toolpolicy.OnceOnly(inner)

	ctx := toolpolicy.LoopScope(context.Background())
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

func TestOnceOnly_DifferentScopesAreIndependent(t *testing.T) {
	inner := &fakeTool{name: "search", resp: "ok"}
	wrapped, _ := toolpolicy.OnceOnly(inner)

	ctx1 := toolpolicy.LoopScope(context.Background())
	ctx2 := toolpolicy.LoopScope(context.Background())

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

func TestOnceOnly_FallbackToProcessWideWithoutScope(t *testing.T) {
	inner := &fakeTool{name: "x", resp: "ok"}
	wrapped, _ := toolpolicy.OnceOnly(inner)

	if _, err := wrapped.Call(context.Background(), `{}`); err != nil {
		t.Fatalf("first call: %v", err)
	}
	_, err := wrapped.Call(context.Background(), `{}`)
	if !errors.Is(err, toolpolicy.ErrToolAlreadyCalled) {
		t.Fatalf("expected ErrToolAlreadyCalled with no scope, got %v", err)
	}
}

func TestOnceOnly_RejectsNilTool(t *testing.T) {
	if _, err := toolpolicy.OnceOnly(nil); err == nil {
		t.Fatal("expected error")
	}
}

// ---------- Unlocked ----------------------------------------------------

func TestUnlocked_LockedReturnsErr(t *testing.T) {
	inner := &fakeTool{name: "delete", resp: "deleted"}
	wrapped, _ := toolpolicy.Unlocked(inner, func(context.Context, string) (bool, string) {
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

func TestUnlocked_UnlockedDelegatesNormally(t *testing.T) {
	inner := &fakeTool{name: "delete", resp: "deleted"}
	wrapped, _ := toolpolicy.Unlocked(inner, func(context.Context, string) (bool, string) {
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

func TestUnlocked_ConditionSeesArguments(t *testing.T) {
	var seen string
	inner := &fakeTool{name: "x", resp: "ok"}
	wrapped, _ := toolpolicy.Unlocked(inner, func(_ context.Context, args string) (bool, string) {
		seen = args
		return true, ""
	})
	wrapped.Call(context.Background(), `{"hello":"world"}`)
	if seen != `{"hello":"world"}` {
		t.Fatalf("condition did not receive args; got %q", seen)
	}
}

func TestUnlocked_RejectsNilArgs(t *testing.T) {
	for _, tc := range []struct {
		name string
		fn   func() error
	}{
		{"nil tool", func() error {
			_, err := toolpolicy.Unlocked(nil, func(context.Context, string) (bool, string) { return true, "" })
			return err
		}},
		{"nil condition", func() error {
			_, err := toolpolicy.Unlocked(&fakeTool{name: "x"}, nil)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fn(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// ---------- Composition ---------------------------------------------------

func TestCompose_OnceOnly_AndUnlock(t *testing.T) {
	inner := &fakeTool{name: "search", resp: "ok"}
	once, err := toolpolicy.OnceOnly(inner)
	if err != nil {
		t.Fatalf("OnceOnly: %v", err)
	}
	gated, err := toolpolicy.Unlocked(once,
		func(context.Context, string) (bool, string) { return true, "" })
	if err != nil {
		t.Fatalf("Unlocked: %v", err)
	}

	ctx := toolpolicy.LoopScope(context.Background())
	if _, err := gated.Call(ctx, `{}`); err != nil {
		t.Fatalf("first composed call: %v", err)
	}
	if _, err := gated.Call(ctx, `{}`); !errors.Is(err, toolpolicy.ErrToolAlreadyCalled) {
		t.Fatalf("expected ErrToolAlreadyCalled on duplicate, got %v", err)
	}
}
