// Package inprocess implements the [transport.Transport] interface
// for "Go ↔ Go in the same binary" deployments — typically a Bubble
// Tea TUI linking the runtime directly. The business path skips
// JSON-RPC serialization entirely; the Transport surface exists only
// so logging / tracing middleware can wrap calls uniformly.
//
// Two modes of use:
//
//  1. Direct Runtime passthrough (recommended). Get the Runtime
//     interface back as-is and call methods directly:
//
//     api := server.New(...)
//     sessions, err := api.ListSessions(ctx, ...)
//
//  2. Through Transport (for middleware symmetry). Wrap the api in
//     an InProcessTransport and treat it like any other transport.
//     Messages are dispatched through delivery/dispatch.Dispatcher so
//     codepaths stay uniform with HTTP.
//
// The second mode is mostly for tests; production TUI code uses #1.
package inprocess

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
)

// messageHandler is the dispatch surface this transport needs: route
// one inbound message, return the synchronous reply plus any stream.
// Defined here (consumer side) so the transport depends on the single
// method it calls rather than the concrete *dispatch.Dispatcher.
type messageHandler interface {
	Handle(ctx context.Context, msg transport.Message, expectedMethod string) dispatch.HandleResult
}

// Transport is the in-process implementation of [transport.Transport].
// Messages are routed through a dispatch.Dispatcher; responses /
// notifications come back via the Recv channel.
type Transport struct {
	dispatcher messageHandler

	in   chan transport.Message // outbound from Runtime's POV → inbound to client
	once sync.Once

	// close signals every sender to stop; gone short-circuits new sends.
	// mu makes "reserve a send slot" and "begin closing" mutually exclusive,
	// and sending counts in-flight sends so Close waits them out BEFORE
	// close(in) — otherwise a send (Send / pumpStream) racing close(in) panics
	// with "send on closed channel" (select doesn't shield a closed-send case).
	close   chan struct{}
	gone    atomic.Bool
	mu      sync.Mutex
	sending sync.WaitGroup
}

// reserve registers one in-flight send unless the transport is closing. On true
// the caller MUST call t.sending.Done() when its send settles; false means the
// transport is closed and the caller must not touch t.in.
func (t *Transport) reserve() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.gone.Load() {
		return false
	}
	t.sending.Add(1)
	return true
}

// Config bundles the inputs for NewTransport.
type Config struct {
	// API is the Runtime implementation the dispatcher routes to.
	// Required.
	Runtime protocol.Runtime

	// RecvBuffer sizes the inbound channel. Defaults to 64. Streaming
	// methods can push many notifications quickly; bigger buffers
	// trade memory for fewer backpressure stalls.
	RecvBuffer int
}

// NewTransport builds an InProcess transport. Returns an error when
// API is nil.
func NewTransport(cfg Config) (*Transport, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("inprocess: API is required")
	}
	if cfg.RecvBuffer <= 0 {
		cfg.RecvBuffer = 64
	}
	return &Transport{
		dispatcher: dispatch.New(cfg.Runtime),
		in:         make(chan transport.Message, cfg.RecvBuffer),
		close:      make(chan struct{}),
	}, nil
}

// Send dispatches one outbound message through the dispatch. For
// streaming methods (runs.start, ...), the resulting events are
// piped onto the Recv channel as notifications/run/event entries.
func (t *Transport) Send(ctx context.Context, msg transport.Message) error {
	if t.gone.Load() {
		return errors.New("inprocess: transport closed")
	}

	res := t.dispatcher.Handle(ctx, msg, "")
	if res.Response != nil {
		if !t.reserve() {
			return errors.New("inprocess: transport closed")
		}
		err := func() error {
			defer t.sending.Done()
			select {
			case t.in <- res.Response:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-t.close:
				return errors.New("inprocess: transport closed")
			}
		}()
		if err != nil {
			return err
		}
	}
	if res.EventStream != nil {
		go t.pumpStream(ctx, res.EventStream)
	}
	return nil
}

// pumpStream drains a streaming method's frame channel and emits each frame's
// pre-encoded notification onto Recv. The dispatch already encoded + tagged
// each frame (run / workspace), so channel close just means "stream done".
// Exits when the channel closes or the transport closes.
func (t *Transport) pumpStream(ctx context.Context, events <-chan dispatch.StreamFrame) {
	for {
		select {
		case frame, ok := <-events:
			if !ok {
				return
			}
			if !t.tryEmit(frame.Notif) {
				return
			}
		case <-ctx.Done():
			return
		case <-t.close:
			return
		}
	}
}

func (t *Transport) tryEmit(msg transport.Message) bool {
	if msg == nil {
		return true
	}
	if !t.reserve() {
		return false
	}
	defer t.sending.Done()
	select {
	case t.in <- msg:
		return true
	case <-t.close:
		return false
	}
}

// Recv returns the inbound channel — responses + notifications.
func (t *Transport) Recv() <-chan transport.Message { return t.in }

// Close signals senders to stop, waits for in-flight sends to settle, then
// closes the Recv channel. Idempotent and safe to call concurrently with Send.
func (t *Transport) Close() error {
	t.once.Do(func() {
		t.mu.Lock()
		t.gone.Store(true)
		close(t.close)
		t.mu.Unlock()
		t.sending.Wait() // no send is mid-flight on t.in past this point
		close(t.in)
	})
	return nil
}
