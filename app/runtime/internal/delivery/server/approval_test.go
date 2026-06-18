package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// TestApprovalModeWireRoundTrip checks every engine stance maps to a wire name
// and back, and that an unknown wire value is rejected (→ invalid_params).
func TestApprovalModeWireRoundTrip(t *testing.T) {
	for _, m := range []approval.Mode{approval.ModeSafe, approval.ModeBalanced, approval.ModeYolo, approval.ModePlan} {
		back, ok := approvalModeFromWire(approvalModeToWire(m))
		if !ok || back != m {
			t.Errorf("round-trip %v → %q → %v (ok=%v)", m, approvalModeToWire(m), back, ok)
		}
	}
	if _, ok := approvalModeFromWire(protocol.ApprovalMode("bogus")); ok {
		t.Error("unknown wire approval mode must be rejected")
	}
}
