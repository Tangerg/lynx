package executionctx

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
)

func TestScopeAccessorsShareOneImmutableTurnValue(t *testing.T) {
	want := execution.TurnScope{
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

func TestModelSelectionUsesRunValueOrRuntimeDefault(t *testing.T) {
	fallback, err := modelref.New("anthropic", "claude-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := modelref.New("openai", "gpt-5")
	if err != nil {
		t.Fatal(err)
	}

	if got := ModelSelection(context.Background(), fallback); got != fallback {
		t.Fatalf("missing Run selection = %+v, want fallback %+v", got, fallback)
	}
	ctx := WithModelSelection(context.Background(), explicit)
	if got := ModelSelection(ctx, fallback); got != explicit {
		t.Fatalf("explicit Run selection = %+v, want %+v", got, explicit)
	}
	ctx = WithModelSelection(context.Background(), modelref.Selection{})
	if got := ModelSelection(ctx, fallback); got != fallback {
		t.Fatalf("zero Run selection = %+v, want fallback %+v", got, fallback)
	}
}
