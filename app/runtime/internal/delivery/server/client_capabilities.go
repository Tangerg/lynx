package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
)

func interruptKindsFromContext(ctx context.Context) []execution.InterruptKind {
	caps, ok := protocol.ClientCapabilitiesFrom(ctx)
	if !ok || len(caps.InterruptTypes) == 0 {
		return nil
	}
	kinds := make([]execution.InterruptKind, 0, len(caps.InterruptTypes))
	for _, kind := range caps.InterruptTypes {
		mapped, ok := interruptKindFromWire(kind)
		if ok {
			kinds = append(kinds, mapped)
		}
	}
	return kinds
}

func interruptKindFromWire(kind protocol.InterruptType) (execution.InterruptKind, bool) {
	switch kind {
	case protocol.InterruptApproval:
		return execution.ApprovalInterrupt, true
	case protocol.InterruptQuestion:
		return execution.QuestionInterrupt, true
	default:
		return 0, false
	}
}
