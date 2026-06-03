package http

import (
	"sync"

	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// clientRegistry routes server→client notifications to the right SSE
// connection per TRANSPORT §8: a notification for a root run goes only
// to the connections that started or subscribed to that run's stream —
// not a blanket broadcast. It holds two maps: connId → live SSE
// connection, and root runId → the set of connIds receiving that run's
// stream. The whole run tree (root + subagent runs, §5.4) rides one root
// stream, so a single subscription covers the subtree.
type clientRegistry struct {
	mu      sync.Mutex
	clients map[string]*clientConn         // connId → live SSE connection
	subs    map[string]map[string]struct{} // root runId → set of connId
}

// clientConn is one active SSE connection. send is the channel the
// HTTP handler drains into the response writer; closing done signals
// the handler to exit.
type clientConn struct {
	send chan transport.Message
	done chan struct{}
	once sync.Once
}

func newClientRegistry() *clientRegistry {
	return &clientRegistry{
		clients: map[string]*clientConn{},
		subs:    map[string]map[string]struct{}{},
	}
}

// register binds connID to its live SSE connection and returns a
// deregister handle. A reconnect under the same connID replaces (and
// closes) the prior connection — one stream per conn id (TRANSPORT §7).
// Subscriptions (runId→connId) are intentionally NOT cleared here: they
// outlive a dropped socket so a Last-Event-Id reconnect under the same
// conn can resume its runs (TRANSPORT §9).
func (r *clientRegistry) register(connID string, c *clientConn) func() {
	r.mu.Lock()
	if old, ok := r.clients[connID]; ok && old != c {
		old.close()
	}
	r.clients[connID] = c
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		if cur, ok := r.clients[connID]; ok && cur == c {
			delete(r.clients, connID)
		}
		r.mu.Unlock()
		c.close()
	}
}

// subscribe registers connID as a receiver of root run runID's stream —
// the routing key the runtime hands back from runs.start / runs.resume /
// runs.subscribe (TRANSPORT §8). Entries are not GC'd on run end: a
// finished run emits no more events, so a stale entry is an inert map key
// (negligible on a local single-user runtime), and keeping it lets a
// just-reconnected conn still replay the terminal events from the buffer.
func (r *clientRegistry) subscribe(runID, connID string) {
	if runID == "" {
		return
	}
	r.mu.Lock()
	conns := r.subs[runID]
	if conns == nil {
		conns = map[string]struct{}{}
		r.subs[runID] = conns
	}
	conns[connID] = struct{}{}
	r.mu.Unlock()
}

// runsForConn returns the root runIds connID is subscribed to — used to
// scope a reconnecting conn's Last-Event-Id replay to its own runs.
func (r *clientRegistry) runsForConn(connID string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for runID, conns := range r.subs {
		if _, ok := conns[connID]; ok {
			out = append(out, runID)
		}
	}
	return out
}

// routeToRun pushes msg to every conn subscribed to root run runID.
// Slow clients drop the message on a full buffer rather than stalling
// the pump for everyone — the replay buffer covers the gap on reconnect.
func (r *clientRegistry) routeToRun(runID string, msg transport.Message) {
	r.mu.Lock()
	var conns []*clientConn
	for connID := range r.subs[runID] {
		if c, ok := r.clients[connID]; ok {
			conns = append(conns, c)
		}
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

// closeAll closes every active client connection and clears routing
// state — called from Server.Shutdown.
func (r *clientRegistry) closeAll() {
	r.mu.Lock()
	conns := make([]*clientConn, 0, len(r.clients))
	for _, c := range r.clients {
		conns = append(conns, c)
	}
	r.clients = map[string]*clientConn{}
	r.subs = map[string]map[string]struct{}{}
	r.mu.Unlock()
	for _, c := range conns {
		c.close()
	}
}

func (c *clientConn) close() {
	c.once.Do(func() { close(c.done) })
}
