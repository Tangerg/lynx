package server

import (
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
)

func presentSafetyClass(class tool.SafetyClass) protocol.SafetyClass {
	switch class {
	case "":
		return ""
	case tool.SafetyClassSafe:
		return protocol.SafetyClassSafe
	case tool.SafetyClassWrite:
		return protocol.SafetyClassWrite
	case tool.SafetyClassExec:
		return protocol.SafetyClassExec
	case tool.SafetyClassNetwork:
		return protocol.SafetyClassNetwork
	default:
		panic("server: unknown tool safety class")
	}
}

func presentApprovalRisk(risk tool.RiskLevel) protocol.ApprovalRisk {
	switch risk {
	case "":
		return ""
	case tool.RiskLow:
		return protocol.ApprovalRiskLow
	case tool.RiskMedium:
		return protocol.ApprovalRiskMedium
	case tool.RiskHigh:
		return protocol.ApprovalRiskHigh
	default:
		panic("server: unknown tool risk level")
	}
}
