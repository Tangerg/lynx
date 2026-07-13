package turn

import (
	"context"
	"fmt"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

// Executor adapts the turn [Dispatcher] to the application's run executor port
// (application/runs.Executor): it drives, observes, and cancels the agent turn
// backing a run segment. The application holds the run lifecycle and drives
// execution through this port, so both the handle it hands back and the events it
// observes are normalized into the application-owned event family. Construct
// via [NewExecutor]; the composition root injects it into the run coordinator.
type Executor struct {
	dispatcher Dispatcher
}

// NewExecutor returns an Executor over the turn dispatcher.
func NewExecutor(dispatcher Dispatcher) *Executor {
	return &Executor{dispatcher: dispatcher}
}

// TurnEvents subscribes to a live turn's event stream. The opaque handle is
// asserted back to the [TurnHandle] the dispatcher minted; each rich turn event
// is translated into the engine-neutral application event contract.
func (e *Executor) TurnEvents(ctx context.Context, handle any) (iter.Seq[runs.EngineEvent], error) {
	h, ok := handle.(TurnHandle)
	if !ok {
		return nil, fmt.Errorf("turn: executor handle %T is not a turn handle", handle)
	}
	seq, err := e.dispatcher.Events(ctx, h)
	if err != nil {
		return nil, err
	}
	return seq, nil
}

// CancelTurn stops a live or parked turn, asserting the opaque handle back to the
// dispatcher's [TurnHandle].
func (e *Executor) CancelTurn(ctx context.Context, handle any) error {
	h, ok := handle.(TurnHandle)
	if !ok {
		return fmt.Errorf("turn: executor handle %T is not a turn handle", handle)
	}
	return e.dispatcher.Cancel(ctx, h)
}
