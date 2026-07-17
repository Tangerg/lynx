package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func presentSafetyClass(class tool.SafetyClass) protocol.SafetyClass {
	switch class {
	case tool.SafetyClassSafe:
		return protocol.SafetyClassSafe
	case tool.SafetyClassWrite:
		return protocol.SafetyClassWrite
	case tool.SafetyClassExec:
		return protocol.SafetyClassExec
	case tool.SafetyClassNetwork:
		return protocol.SafetyClassNetwork
	default:
		return ""
	}
}

func presentApprovalRisk(risk tool.RiskLevel) protocol.ApprovalRisk {
	switch risk {
	case tool.RiskLow:
		return protocol.ApprovalRiskLow
	case tool.RiskMedium:
		return protocol.ApprovalRiskMedium
	case tool.RiskHigh:
		return protocol.ApprovalRiskHigh
	default:
		return ""
	}
}
