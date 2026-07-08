package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
)

type interruptList interface {
	List(ctx context.Context, sessionID string) ([]interrupts.Pending, error)
}

// ListPendingInterrupts returns durable open HITL interrupts. Empty sessionID
// returns every pending interrupt.
func (r *Runtime) ListPendingInterrupts(ctx context.Context, sessionID string) ([]interrupts.Pending, error) {
	return r.interruptList.List(ctx, sessionID)
}
