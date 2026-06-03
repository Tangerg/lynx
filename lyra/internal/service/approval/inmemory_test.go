package approval_test

import (
	"context"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/service/approval"
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
