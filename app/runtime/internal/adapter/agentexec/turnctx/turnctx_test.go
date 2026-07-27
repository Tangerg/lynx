package turnctx

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestScopeAccessorsShareOneImmutableTurnValue(t *testing.T) {
	want := execution.TurnScope{
		SessionID:   "session-1",
		Cwd:         "/workspace/project",
		Isolated:    true,
		GoalLeaseID: "lease-1",
	}
	ctx := WithScope(context.Background(), want)

	if got, ok := ScopeFrom(ctx); !ok || got != want {
		t.Fatalf("ScopeFrom = (%+v, %v), want %+v", got, ok, want)
	}
	if got := TurnSession(ctx); got != want.SessionID {
		t.Fatalf("TurnSession = %q, want %q", got, want.SessionID)
	}
	if got := TurnCwd(ctx, "/fallback"); got != want.Cwd {
		t.Fatalf("TurnCwd = %q, want %q", got, want.Cwd)
	}
	if !TurnIsolated(ctx) {
		t.Fatal("TurnIsolated = false, want true")
	}
	if got, ok := TurnGoalLease(ctx); !ok || got != want.GoalLeaseID {
		t.Fatalf("TurnGoalLease = (%q, %v), want (%q, true)", got, ok, want.GoalLeaseID)
	}
}

func TestMissingScopeUsesHostFallbacks(t *testing.T) {
	ctx := context.Background()
	if _, ok := ScopeFrom(ctx); ok {
		t.Fatal("ScopeFrom unexpectedly found a scope")
	}
	if got := TurnCwd(ctx, "/fallback"); got != "/fallback" {
		t.Fatalf("TurnCwd = %q, want fallback", got)
	}
	if TurnSession(ctx) != "" || TurnIsolated(ctx) {
		t.Fatal("missing scope produced session or isolation")
	}
	if _, ok := TurnGoalLease(ctx); ok {
		t.Fatal("missing scope produced a goal lease")
	}
}
