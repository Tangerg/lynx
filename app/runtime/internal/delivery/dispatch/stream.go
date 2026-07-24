package dispatch

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// StreamFrame is one ready-to-write downstream notification on a streaming
// method's event channel. The dispatch produces these from domain events so
// every transport writes them identically. SSEID drives Last-Event-Id replay;
// "" marks an ephemeral frame (no replay) — e.g. all workspace events.
type StreamFrame struct {
	Notif transport.Message
	SSEID string
}

// adaptStream fans a source sequence into a StreamFrame channel via conv, which
// encodes each event (returns ok=false to skip an unencodable one). The source
// blocks in its own next between events and is unwound by request-context
// cancellation (the run subscription aborts, the workspace channel closes), so
// the range ends; the output send stays ctx-guarded so a stalled downstream
// cannot strand the goroutine. Leak-safe: the streaming request's ctx ends on
// client disconnect / completion, which stops both the source and this send.
func adaptStream[T any](ctx context.Context, in iter.Seq[T], conv func(T) (StreamFrame, bool)) <-chan StreamFrame {
	out := make(chan StreamFrame)
	go func() {
		defer close(out)
		for ev := range in {
			frame, ok := conv(ev)
			if !ok {
				continue
			}
			select {
			case out <- frame:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// chanSeq adapts a channel source to the iter.Seq adaptStream consumes. Used for
// the lossy workspace fan-out, whose channel SubscribeWorkspace closes on
// request-context cancellation, so the range terminates.
func chanSeq[T any](ch <-chan T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range ch {
			if !yield(v) {
				return
			}
		}
	}
}
