package http

import (
	"sync"

	"github.com/Tangerg/lynx/lyra/pkg/transport"
)

// clientRegistry tracks every connected SSE consumer so the
// HTTP transport can fan a single outbound notification out to
// every active /v1/rpc/stream listener. Today's MVP runs as a
// single-tenant in-process Runtime — broadcasting to every client
// is the right semantics (everyone sees the same world).
//
// Per-stream affinity (only sending an event to the client that
// started the run) is a future enhancement; the protocol's
// streamHandle already encodes the routing key when we want it.
type clientRegistry struct {
	mu      sync.Mutex
	clients map[*clientConn]struct{}
}

// clientConn is one active SSE connection. send is the channel the
// HTTP handler drains into the response writer; closing done signals
// the handler to exit.
type clientConn struct {
	send chan *transport.Message
	done chan struct{}
	once sync.Once
}

func newClientRegistry() *clientRegistry {
	return &clientRegistry{clients: map[*clientConn]struct{}{}}
}

// register adds one client and returns a deregister handle.
func (r *clientRegistry) register(c *clientConn) func() {
	r.mu.Lock()
	r.clients[c] = struct{}{}
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		delete(r.clients, c)
		r.mu.Unlock()
		c.close()
	}
}

// broadcast pushes msg into every client's send buffer. Slow clients
// drop the message on a full buffer rather than blocking the
// runtime — keeps one stuck SSE consumer from stalling everyone else.
func (r *clientRegistry) broadcast(msg *transport.Message) {
	r.mu.Lock()
	conns := make([]*clientConn, 0, len(r.clients))
	for c := range r.clients {
		conns = append(conns, c)
	}
	r.mu.Unlock()
	for _, c := range conns {
		select {
		case c.send <- msg:
		default:
			// buffer full — drop. The replay buffer covers reconnects.
		}
	}
}

// closeAll closes every active client connection — called from
// Server.Shutdown.
func (r *clientRegistry) closeAll() {
	r.mu.Lock()
	conns := make([]*clientConn, 0, len(r.clients))
	for c := range r.clients {
		conns = append(conns, c)
	}
	r.clients = map[*clientConn]struct{}{}
	r.mu.Unlock()
	for _, c := range conns {
		c.close()
	}
}

func (c *clientConn) close() {
	c.once.Do(func() { close(c.done) })
}
