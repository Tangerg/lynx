package approval_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// TestModeGetSet verifies the runtime stance round-trips through
// GetMode / SetMode.
func TestModeGetSet(t *testing.T) {
	svc := approval.New(approval.ModeYolo)
	if m, _ := svc.GetMode(context.Background()); m != approval.ModeYolo {
		t.Fatalf("initial mode = %v, want Yolo", m)
	}
	if err := svc.SetMode(context.Background(), approval.ModeBalanced); err != nil {
		t.Fatalf("SetMode: %v", err)
	}
	if m, _ := svc.GetMode(context.Background()); m != approval.ModeBalanced {
		t.Fatalf("mode after set = %v, want Balanced", m)
	}
}

// TestRemember covers the standing per-session decisions the approval gate
// consults: a tool with no record is a miss; a remembered approve / deny each
// round-trip its verdict; the record is scoped per (session, tool) so it never
// leaks across sessions or tools.
func TestRemember(t *testing.T) {
	ctx := context.Background()
	svc := approval.New(approval.ModeSafe)

	// Unseen tool → not remembered.
	if _, ok, _ := svc.Remembered(ctx, "s1", "bash"); ok {
		t.Fatal("Remembered(unseen) ok = true, want false")
	}

	// Remembered approve → ok, approved.
	_ = svc.Remember(ctx, "s1", "bash", true)
	if approved, ok, _ := svc.Remembered(ctx, "s1", "bash"); !ok || !approved {
		t.Fatalf("remembered approve = (%v,%v), want (true,true)", approved, ok)
	}

	// Remembered deny → ok, NOT approved (a denial is a valid standing decision).
	_ = svc.Remember(ctx, "s1", "write", false)
	if approved, ok, _ := svc.Remembered(ctx, "s1", "write"); !ok || approved {
		t.Fatalf("remembered deny = (%v,%v), want (false,true)", approved, ok)
	}

	// Scoped per (session, tool): same tool, other session is still a miss.
	if _, ok, _ := svc.Remembered(ctx, "s2", "bash"); ok {
		t.Fatal("remembered leaked across sessions")
	}

	// Empty session / tool is a no-op (never recorded).
	_ = svc.Remember(ctx, "", "bash", true)
	if _, ok, _ := svc.Remembered(ctx, "", "bash"); ok {
		t.Fatal("empty session recorded a decision")
	}
}
