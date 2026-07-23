package bootstrap

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/adapter/agentexec/turn"
)

// runSegmentProcesses resolves a parked turn's persisted process id from the turn
// dispatcher — the recoverable process id the interrupt commit records.
type runSegmentProcesses struct {
	dispatcher turnProcessLookup
}

type turnProcessLookup interface {
	ProcessID(context.Context, turn.TurnHandle) (string, error)
}

func (p runSegmentProcesses) ProcessID(ctx context.Context, handle turn.TurnHandle) (string, error) {
	return p.dispatcher.ProcessID(ctx, handle)
}
