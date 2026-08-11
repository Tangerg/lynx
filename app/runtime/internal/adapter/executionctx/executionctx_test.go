package executionctx

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

func TestScopeAccessorsShareOneImmutableTurnValue(t *testing.T) {
	want := runs.ExecutionScope{
		SessionID:         "session-1",
		CWD:               "/sandbox/project",
		WorkspaceCWD:      "/workspace/project",
		Isolated:          true,
		GoalIncarnationID: "lease-1",
	}
	ctx := WithScope(context.Background(), want)

	if got, ok := Scope(ctx); !ok || got != want {
		t.Fatalf("Scope = (%+v, %v), want %+v", got, ok, want)
	}
	if got := SessionID(ctx); got != want.SessionID {
		t.Fatalf("SessionID = %q, want %q", got, want.SessionID)
	}
	if got := CWD(ctx, "/fallback"); got != want.CWD {
		t.Fatalf("CWD = %q, want %q", got, want.CWD)
	}
	if got := WorkspaceCWD(ctx, "/fallback"); got != want.WorkspaceCWD {
		t.Fatalf("WorkspaceCWD = %q, want %q", got, want.WorkspaceCWD)
	}
	if !Isolated(ctx) {
		t.Fatal("Isolated = false, want true")
	}
	if got, ok := GoalIncarnationID(ctx); !ok || got != want.GoalIncarnationID {
		t.Fatalf("GoalIncarnationID = (%q, %v), want (%q, true)", got, ok, want.GoalIncarnationID)
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
	if _, ok := GoalIncarnationID(ctx); ok {
		t.Fatal("missing scope produced a goal incarnation")
	}
}
