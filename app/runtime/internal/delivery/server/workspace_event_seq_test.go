package server

import (
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// drainSeq bridges a workspace subscription's iter.Seq back to a channel so a
// test can select the next event, its close, or a timeout. These select-based
// assertions are a test concern, not a production one (production consumes the
// unified EventStream). The goroutine ends when the sequence does — callers
// cancel their request context, which closes the subscription's source.
func drainSeq(seq iter.Seq[protocol.WorkspaceEvent]) <-chan protocol.WorkspaceEvent {
	ch := make(chan protocol.WorkspaceEvent)
	go func() {
		defer close(ch)
		for ev := range seq {
			ch <- ev
		}
	}()
	return ch
}
