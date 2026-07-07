package dispatch

import (
	"context"

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

// adaptStream fans a domain event channel into a StreamFrame channel via conv,
// which encodes each event (returns ok=false to skip an unencodable one). The
// goroutine exits on ctx cancellation OR when in closes, and never blocks past
// ctx (leak-safe): the streaming request's ctx ends on client disconnect /
// completion, which also stops the source.
func adaptStream[T any](ctx context.Context, in <-chan T, conv func(T) (StreamFrame, bool)) <-chan StreamFrame {
	out := make(chan StreamFrame)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-in:
				if !ok {
					return
				}
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
		}
	}()
	return out
}
