package executionctx

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func TestScopeAccessorsShareOneImmutableTurnValue(t *testing.T) {
	want := execution.ExecutionScope{
		SessionID:    "session-1",
		Cwd:          "/sandbox/project",
		WorkspaceCwd: "/workspace/project",
		Isolated:     true,
		GoalLeaseID:  "lease-1",
	}
	ctx := WithScope(context.Background(), want)

	if got, ok := Scope(ctx); !ok || got != want {
		t.Fatalf("Scope = (%+v, %v), want %+v", got, ok, want)
	}
	if got := SessionID(ctx); got != want.SessionID {
		t.Fatalf("SessionID = %q, want %q", got, want.SessionID)
	}
	if got := CWD(ctx, "/fallback"); got != want.Cwd {
		t.Fatalf("CWD = %q, want %q", got, want.Cwd)
	}
	if got := WorkspaceCWD(ctx, "/fallback"); got != want.WorkspaceCwd {
		t.Fatalf("WorkspaceCWD = %q, want %q", got, want.WorkspaceCwd)
	}
	if !Isolated(ctx) {
		t.Fatal("Isolated = false, want true")
	}
	if got, ok := GoalLeaseID(ctx); !ok || got != want.GoalLeaseID {
		t.Fatalf("GoalLeaseID = (%q, %v), want (%q, true)", got, ok, want.GoalLeaseID)
	}
}

func TestMissingScopeUsesHostFallbacks(t *testing.T) {
	ctx := context.Background()
	if _, ok := Scope(ctx); ok {
		t.Fatal("Scope unexpectedly found a scope")
	}
	if got := CWD(ctx, "/fallback"); got != "/fallback" {
		t.Fatalf("CWD = %q, want fallback", got)
	}
	if got := WorkspaceCWD(ctx, "/fallback"); got != "/fallback" {
		t.Fatalf("WorkspaceCWD = %q, want fallback", got)
	}
	if SessionID(ctx) != "" || Isolated(ctx) {
		t.Fatal("missing scope produced session or isolation")
	}
	if _, ok := GoalLeaseID(ctx); ok {
		t.Fatal("missing scope produced a goal lease")
	}
}
