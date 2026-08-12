package executionctx

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
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

func TestRunCapabilitiesAreOwnershipIsolated(t *testing.T) {
	input := run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}
	ctx := WithRunCapabilities(context.Background(), input)
	input.InterruptKinds[0] = interrupt.Approval
	first, ok := RunCapabilities(ctx)
	if !ok || !first.Equal(run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}) {
		t.Fatalf("RunCapabilities = (%+v, %t)", first, ok)
	}
	first.InterruptKinds[0] = interrupt.Approval
	second, _ := RunCapabilities(ctx)
	if !second.Equal(run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}}) {
		t.Fatalf("RunCapabilities returned shared storage: %+v", second)
	}
}
