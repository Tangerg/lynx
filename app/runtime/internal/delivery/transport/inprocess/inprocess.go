// Package inprocess provides the concrete JSON-RPC transport for same-process
// clients such as a CLI/TUI that embeds the runtime instead of talking over
// HTTP. The runtime server binary does not use this package.
//
// Two modes of use:
//
//  1. Direct Runtime passthrough. Get the Runtime interface back as-is and call
//     methods directly:
//
//     api := server.New(...)
//     sessions, err := api.ListSessions(ctx, ...)
//
//  2. Through Transport. Wrap the api in an InProcessTransport and treat it like
//     any other transport. Messages are dispatched through
//     delivery/dispatch.Router so codepaths stay uniform with HTTP.
package inprocess

import (
	"context"
	"errors"
	"iter"
	"sync"
	"sync/atomic"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/transport"
	"github.com/Tangerg/lynx/app/runtime/internal/taskgroup"
)

var errTransportClosed = errors.New("inprocess: transport closed")

// messageHandler is the dispatch surface this transport needs: route
// one inbound message, return the synchronous reply plus any stream.
// Defined here (consumer side) so the transport depends on the single
// method it calls rather than the concrete *dispatch.Router.
type messageHandler interface {
	Handle(ctx context.Context, msg transport.Message) dispatch.HandleResult
}

// Transport routes in-process JSON-RPC messages through a dispatch.Router;
// responses and notifications come back via the Recv channel.
type Transport struct {
	router messageHandler

	in   chan transport.Message // outbound from Runtime's POV -> inbound to client
	once sync.Once

	// close signals every sender to stop; gone short-circuits new sends.
	// mu makes "reserve a send slot" and "begin closing" mutually exclusive,
	// and sending counts in-flight sends so AwaitShutdown closes in only after
	// every reserved sender has drained.
	close   chan struct{}
	gone    atomic.Bool
	mu      sync.Mutex
	sending sync.WaitGroup
	pumps   sync.WaitGroup
	calls   taskgroup.Group
	done    chan struct{}
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
	// Runtime is the protocol implementation the router routes to.
	// Required.
	Runtime protocol.Runtime

	// RecvBuffer sizes the inbound channel. Defaults to 64. Streaming
	// methods can push many notifications quickly; bigger buffers
	// trade memory for fewer backpressure stalls.
	RecvBuffer int
}

// NewTransport builds an InProcess transport. Returns an error when
// Runtime is nil.
func NewTransport(cfg Config) (*Transport, error) {
	if cfg.Runtime == nil {
		return nil, errors.New("inprocess: Runtime is required")
	}
	if cfg.RecvBuffer <= 0 {
		cfg.RecvBuffer = 64
	}
	return &Transport{
		router: dispatch.New(cfg.Runtime, dispatch.Config{}),
		in:     make(chan transport.Message, cfg.RecvBuffer),
		close:  make(chan struct{}),
		done:   make(chan struct{}),
	}, nil
}

// Send dispatches one outbound message through the dispatch. For
// streaming methods (runs.start, ...), the resulting events are
// piped onto the Recv channel as notifications.run.event entries.
func (t *Transport) Send(ctx context.Context, msg transport.Message) error {
	callCtx, release, ok := t.calls.AttachLinked(ctx)
	if !ok {
		return errTransportClosed
	}
	releaseCall := true
	defer func() {
		if releaseCall {
			release()
		}
	}()
	res := t.router.Handle(callCtx, msg)
	if res.Response != nil {
		if !t.reserve() {
			return errTransportClosed
		}
		err := func() error {
			defer t.sending.Done()
			select {
			case t.in <- res.Response:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			case <-t.close:
				return errTransportClosed
			}
		}()
		if err != nil {
			return err
		}
	}
	if res.EventStream != nil {
		if !t.startPump(callCtx, res.EventStream, release) {
			return errTransportClosed
		}
		releaseCall = false // the stream pump now owns the attached call
	}
	return nil
}

func (t *Transport) startPump(ctx context.Context, events iter.Seq[dispatch.StreamFrame], release func()) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.gone.Load() {
		return false
	}
	t.pumps.Add(1)
	go func() {
		defer release()
		defer t.pumps.Done()
		t.pumpStream(ctx, events)
	}()
	return true
}

// pumpStream ranges a streaming method's frame sequence and emits each frame's
// pre-encoded notification onto Recv. The dispatch already encoded + tagged each
// frame (run / workspace), so the sequence ending just means "stream done". The
// source unwinds when the call context is canceled — including on transport
// BeginShutdown, since the call context is task-group-linked (AttachLinked) — so a range
// blocked between frames is released; tryEmit guards the send against ctx and
// transport close.
func (t *Transport) pumpStream(ctx context.Context, events iter.Seq[dispatch.StreamFrame]) {
	for frame := range events {
		if !t.tryEmit(ctx, frame.Notif) {
			return
		}
	}
}

func (t *Transport) tryEmit(ctx context.Context, msg transport.Message) bool {
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
	case <-ctx.Done():
		return false
	case <-t.close:
		return false
	}
}

// Recv returns the inbound channel — responses + notifications.
func (t *Transport) Recv() <-chan transport.Message { return t.in }

// BeginShutdown rejects new sends and cancels every attached call. It is safe
// to invoke repeatedly and does not wait, so a process owner can broadcast
// cancellation across all of its components before draining any one of them.
func (t *Transport) BeginShutdown() {
	t.once.Do(func() {
		t.mu.Lock()
		t.gone.Store(true)
		close(t.close)
		t.mu.Unlock()
		t.calls.Cancel()
		go func() {
			_ = t.calls.Wait(context.Background())
			t.pumps.Wait()
			t.sending.Wait()
			close(t.in)
			close(t.done)
		}()
	})
}

// AwaitShutdown waits until all calls, streams, and in-flight sends have
// drained and Recv is closed. The caller supplies the deadline; a timeout is
// observable and a later await may continue waiting for the same shutdown.
func (t *Transport) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("inprocess: shutdown context is required")
	}
	t.BeginShutdown()
	select {
	case <-t.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
