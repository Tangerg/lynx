package server

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// drainSeq bridges a workspace subscription's iter.Seq back to a channel so a
// test can select the next event, its close, or a timeout. These select-based
// assertions are a test concern, not a production one. ctx also makes the test
// bridge stoppable while it is waiting to hand an event to the assertion.
func drainSeq(ctx context.Context, seq iter.Seq[protocol.WorkspaceEvent]) <-chan protocol.WorkspaceEvent {
	ch := make(chan protocol.WorkspaceEvent)
	go func() {
		defer close(ch)
		for ev := range seq {
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}
