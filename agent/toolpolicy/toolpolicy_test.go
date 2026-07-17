package toolpolicy_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/agent/toolpolicy"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/tools"
)

type fakeTool struct {
	name    string
	calls   int
	gotArgs []string
	resp    string
	respErr error
	direct  bool
	paths   []string
	pathErr error
}

func (t *fakeTool) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{Name: t.name, InputSchema: json.RawMessage(`{"type":"object"}`)}
}
func (t *fakeTool) Call(_ context.Context, args string) (string, error) {
	t.calls++
	t.gotArgs = append(t.gotArgs, args)
	return t.resp, t.respErr
}
func (t *fakeTool) ReturnsDirect() bool { return t.direct }
func (t *fakeTool) ConcurrencyKey(string) (string, bool) {
	return "resource", true
}
func (t *fakeTool) MutationPaths(string) ([]string, error) {
	return slices.Clone(t.paths), t.pathErr
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

func TestOnceOnly_ForwardsToolCapabilities(t *testing.T) {
	inner := &fakeTool{name: "notify", direct: true, paths: []string{"message.txt"}}
	wrapped, err := toolpolicy.OnceOnly(inner)
	if err != nil {
		t.Fatalf("OnceOnly: %v", err)
	}

	assertPolicyCapabilities(t, wrapped, []string{"message.txt"})
	if _, err := wrapped.Call(toolpolicy.LoopScope(t.Context()), `{}`); err != nil {
		t.Fatalf("Call after metadata inspection: %v", err)
	}
	if inner.calls != 1 {
		t.Fatalf("delegate calls after metadata inspection = %d, want 1", inner.calls)
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

func TestUnlocked_ForwardsToolCapabilities(t *testing.T) {
	inner := &fakeTool{name: "notify", direct: true, paths: []string{"message.txt"}}
	conditionCalls := 0
	wrapped, err := toolpolicy.Unlocked(inner, func(context.Context, string) (bool, string) {
		conditionCalls++
		return true, ""
	})
	if err != nil {
		t.Fatalf("Unlocked: %v", err)
	}

	assertPolicyCapabilities(t, wrapped, []string{"message.txt"})
	if conditionCalls != 0 {
		t.Fatalf("metadata inspection evaluated unlock condition %d times", conditionCalls)
	}
	if _, err := wrapped.Call(t.Context(), `{}`); err != nil {
		t.Fatalf("Call after metadata inspection: %v", err)
	}
	if conditionCalls != 1 || inner.calls != 1 {
		t.Fatalf("condition/delegate calls = %d/%d, want 1/1", conditionCalls, inner.calls)
	}
}

func assertPolicyCapabilities(t *testing.T, wrapped tools.Tool, wantPaths []string) {
	t.Helper()
	direct, ok := wrapped.(interface{ ReturnsDirect() bool })
	if !ok || !direct.ReturnsDirect() {
		t.Fatal("return-direct capability was not forwarded")
	}
	reporter, ok := wrapped.(tools.FileMutationReporter)
	if !ok {
		t.Fatal("file-mutation capability was not forwarded")
	}
	paths, err := reporter.MutationPaths(`{"path":"message.txt"}`)
	if err != nil || !slices.Equal(paths, wantPaths) {
		t.Fatalf("MutationPaths() = %v, %v; want %v, nil", paths, err, wantPaths)
	}
	if _, concurrent := wrapped.(interface {
		ConcurrencyKey(string) (string, bool)
	}); concurrent {
		t.Fatal("stateful policy must keep the wrapped tool exclusive")
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
