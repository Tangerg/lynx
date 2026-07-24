package runs

import (
	"context"
	"iter"
)

// seqToEventChan is transitional migration scaffolding for the Run-event chain's
// channel→iter.Seq conversion. It adapts the Journal's iter.Seq back to the
// <-chan Event the not-yet-migrated downstream stages still consume, re-creating
// the per-subscriber pump goroutine that the fully-migrated pipeline deletes.
// Across the migration this bridge moves one stage outward each batch until it
// reaches the HTTP transport — its permanent home, where selecting against
// heartbeat and cancellation genuinely needs a channel — and vanishes from the
// in-process path. Removed once the application stream type becomes iter.Seq.
//
// The caller wires the source subscription's cancel to ctx (context.AfterFunc),
// which wakes a range blocked waiting for the next event; the select here covers
// the other block point — a stalled downstream — on the same ctx.
func seqToEventChan(ctx context.Context, seq iter.Seq[Event]) <-chan Event {
	out := make(chan Event)
	go func() {
		defer close(out)
		for ev := range seq {
			select {
			case out <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
