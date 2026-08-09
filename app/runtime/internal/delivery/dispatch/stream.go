package dispatch

import (
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// StreamFrame is one ready-to-write downstream notification on a streaming
// method's event stream. The dispatch produces these from domain events so
// every transport writes them identically. SSEID drives Last-Event-Id replay;
// "" marks an ephemeral frame (no replay) — e.g. all workspace events.
type StreamFrame struct {
	Notification transport.Message
	SSEID        string
}

// adaptStream maps a source sequence into a StreamFrame sequence. The encoder
// returns ok=false to skip an unencodable event. It is a pure
// synchronous transform: it runs on the consumer's goroutine and holds no
// channel of its own. Cancellation is the source's and the consumer's concern —
// the source unwinds on request-context cancellation and the final transport
// stops ranging — so nothing here can leak.
func adaptStream[T any](events iter.Seq[T], encode func(T) (StreamFrame, bool)) iter.Seq[StreamFrame] {
	return func(yield func(StreamFrame) bool) {
		for event := range events {
			frame, ok := encode(event)
			if !ok {
				continue
			}
			if !yield(frame) {
				return
			}
		}
	}
}

// emptyStream is a non-nil, immediately-ending frame sequence: a streaming
// method whose stream is already over (distinct from a nil EventStream, which
// marks a non-streaming method).
func emptyStream(func(StreamFrame) bool) {}
