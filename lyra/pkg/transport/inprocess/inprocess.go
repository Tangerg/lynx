// Package inprocess implements the [transport.Transport] interface
// for "Go ↔ Go in the same binary" deployments — typically a Bubble
// Tea TUI linking the runtime directly. The business path skips
// JSON-RPC serialisation entirely; the Transport surface exists only
// so logging / tracing middleware can wrap calls uniformly.
//
// Two modes of use:
//
//  1. Direct CoreAPI passthrough (recommended). Get the CoreAPI
//     interface back as-is and call methods directly:
//
//	    api := coreimpl.New(...)
//	    sessions, err := api.ListSessions(ctx, ...)
//
//  2. Through Transport (for middleware symmetry). Wrap the api in
//     an InProcessTransport and treat it like any other transport.
//     Messages are dispatched through pkg/rpcadapter.Dispatcher so
//     codepaths stay uniform with HTTP/Wails.
//
// The second mode is mostly for tests; production TUI code uses #1.
package inprocess

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/lyra/pkg/coreapi"
	"github.com/Tangerg/lynx/lyra/pkg/rpcadapter"
	"github.com/Tangerg/lynx/lyra/pkg/transport"
)

// Transport is the in-process implementation of [transport.Transport].
// Messages are routed through a rpcadapter.Dispatcher; responses /
// notifications come back via the Recv channel.
type Transport struct {
	dispatcher *rpcadapter.Dispatcher

	in    chan *transport.Message // outbound from Runtime's POV → inbound to client
	once  sync.Once
	close chan struct{}
	gone  atomic.Bool
}

// Config bundles the inputs for NewTransport.
type Config struct {
	// API is the CoreAPI implementation the dispatcher routes to.
	// Required.
	API coreapi.CoreAPI

	// RecvBuffer sizes the inbound channel. Defaults to 64. Streaming
	// methods can push many notifications quickly; bigger buffers
	// trade memory for fewer backpressure stalls.
	RecvBuffer int
}

// NewTransport builds an InProcess transport. Returns an error when
// API is nil.
func NewTransport(cfg Config) (*Transport, error) {
	if cfg.API == nil {
		return nil, errors.New("inprocess: API is required")
	}
	if cfg.RecvBuffer <= 0 {
		cfg.RecvBuffer = 64
	}
	return &Transport{
		dispatcher: rpcadapter.New(cfg.API),
		in:         make(chan *transport.Message, cfg.RecvBuffer),
		close:      make(chan struct{}),
	}, nil
}

// Send dispatches one outbound message through the rpcadapter. For
// streaming methods (runs.start, ...), the resulting events are
// piped onto the Recv channel as notifications/run/event entries.
func (t *Transport) Send(ctx context.Context, msg *transport.Message) error {
	if t.gone.Load() {
		return errors.New("inprocess: transport closed")
	}

	res := t.dispatcher.Handle(ctx, msg, "")
	if res.Response != nil {
		select {
		case t.in <- res.Response:
		case <-ctx.Done():
			return ctx.Err()
		case <-t.close:
			return errors.New("inprocess: transport closed")
		}
	}
	if res.EventStream != nil {
		go t.pumpStream(ctx, res.RunID, res.EventStream)
	}
	return nil
}

// pumpStream drains an event channel from a streaming method and
// encodes each event as a notifications/run/event message. Exits
// when the channel closes (run ended) or the transport closes.
// runID is the resource id used for stream filtering (API.md v4 §3.1).
func (t *Transport) pumpStream(ctx context.Context, runID string, events <-chan coreapi.AgUiEvent) {
	var seq uint64
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				closedMsg, _ := rpcadapter.EncodeRunClosed(runID, "completed")
				t.tryEmit(closedMsg)
				return
			}
			seq++
			notif, err := rpcadapter.EncodeRunEvent(runID, formatSeq(seq), ev)
			if err != nil {
				continue
			}
			if !t.tryEmit(notif) {
				return
			}
		case <-ctx.Done():
			return
		case <-t.close:
			return
		}
	}
}

func (t *Transport) tryEmit(msg *transport.Message) bool {
	if msg == nil {
		return true
	}
	select {
	case t.in <- msg:
		return true
	case <-t.close:
		return false
	}
}

// Recv returns the inbound channel — responses + notifications.
func (t *Transport) Recv() <-chan *transport.Message { return t.in }

// Close drains pending sends and closes the Recv channel. Idempotent.
func (t *Transport) Close() error {
	t.once.Do(func() {
		t.gone.Store(true)
		close(t.close)
		close(t.in)
	})
	return nil
}

// formatSeq is a small helper that turns a uint64 into a stable
// string for use as eventId. We use decimal so Last-Event-Id resume
// can compare numerically without a separate parse step.
func formatSeq(n uint64) string {
	// 20 chars max for uint64 — small enough to alloc on the stack
	// inside Itoa wrappers.
	var buf [20]byte
	pos := len(buf)
	if n == 0 {
		pos--
		buf[pos] = '0'
	}
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
