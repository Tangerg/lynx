package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func TestPresentSafetyValues(t *testing.T) {
	for _, test := range []struct {
		domain tool.SafetyClass
		wire   protocol.SafetyClass
	}{
		{domain: tool.SafetyClassSafe, wire: protocol.SafetyClassSafe},
		{domain: tool.SafetyClassWrite, wire: protocol.SafetyClassWrite},
		{domain: tool.SafetyClassExec, wire: protocol.SafetyClassExec},
		{domain: tool.SafetyClassNetwork, wire: protocol.SafetyClassNetwork},
		{domain: "", wire: ""},
		{domain: "future", wire: ""},
	} {
		if got := presentSafetyClass(test.domain); got != test.wire {
			t.Errorf("presentSafetyClass(%q) = %q, want %q", test.domain, got, test.wire)
		}
	}

	for _, test := range []struct {
		domain tool.RiskLevel
		wire   protocol.ApprovalRisk
	}{
		{domain: tool.RiskLow, wire: protocol.ApprovalRiskLow},
		{domain: tool.RiskMedium, wire: protocol.ApprovalRiskMedium},
		{domain: tool.RiskHigh, wire: protocol.ApprovalRiskHigh},
		{domain: "", wire: ""},
		{domain: "future", wire: ""},
	} {
		if got := presentApprovalRisk(test.domain); got != test.wire {
			t.Errorf("presentApprovalRisk(%q) = %q, want %q", test.domain, got, test.wire)
		}
	}
}
