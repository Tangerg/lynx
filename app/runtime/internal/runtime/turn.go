package runtime

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// TurnProcessID returns the persisted agent-process id backing a parked turn.
func (r *Runtime) TurnProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return r.turns.ProcessID(ctx, handle)
}
