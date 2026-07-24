package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func interruptKindsFromContext(ctx context.Context) []runs.InterruptKind {
	caps, ok := protocol.ClientCapabilitiesFrom(ctx)
	if !ok || len(caps.InterruptTypes) == 0 {
		return nil
	}
	kinds := make([]runs.InterruptKind, 0, len(caps.InterruptTypes))
	for _, kind := range caps.InterruptTypes {
		mapped, ok := interruptKindFromWire(kind)
		if ok {
			kinds = append(kinds, mapped)
		}
	}
	return kinds
}

func interruptKindFromWire(kind protocol.InterruptType) (runs.InterruptKind, bool) {
	switch kind {
	case protocol.InterruptApproval:
		return runs.ApprovalInterruptKind, true
	case protocol.InterruptQuestion:
		return runs.QuestionInterruptKind, true
	default:
		return "", false
	}
}
