package http

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/lyra/rpc/transport"
)

// newConn is a tiny clientConn with a buffered send channel so
// routeToRun never blocks in these single-threaded tests.
func newConn() *clientConn {
	return &clientConn{send: make(chan transport.Message, 4), done: make(chan struct{})}
}

// testNotif builds a throwaway notification message for routing tests.
func testNotif() transport.Message {
	msg, err := transport.NewNotification("notifications.run.event", nil)
	if err != nil {
		panic(err)
	}
	return msg
}

// TestRouteToRunOnlyHitsSubscribers confirms a run's events reach the
// conns subscribed to that root run and no others (TRANSPORT §8) —
// the routing that replaced the old blanket broadcast.
func TestRouteToRunOnlyHitsSubscribers(t *testing.T) {
	reg := newClientRegistry()
	a, b := newConn(), newConn()
	reg.register("conn-a", a)
	reg.register("conn-b", b)

	// Only conn-a subscribes to run_1.
	reg.subscribe("run_1", "conn-a")

	msg := testNotif()
	reg.routeToRun("run_1", msg)

	if len(a.send) != 1 {
		t.Fatalf("conn-a got %d msgs, want 1", len(a.send))
	}
	if len(b.send) != 0 {
		t.Fatalf("conn-b got %d msgs, want 0 (not subscribed)", len(b.send))
	}
}

// TestRouteToRunFanOut confirms multiple conns subscribed to the same
// root run (e.g. two tabs that both runs.subscribe'd) each receive it.
func TestRouteToRunFanOut(t *testing.T) {
	reg := newClientRegistry()
	a, b := newConn(), newConn()
	reg.register("conn-a", a)
	reg.register("conn-b", b)
	reg.subscribe("run_1", "conn-a")
	reg.subscribe("run_1", "conn-b")

	reg.routeToRun("run_1", testNotif())

	if len(a.send) != 1 || len(b.send) != 1 {
		t.Fatalf("fan-out failed: a=%d b=%d, want 1 1", len(a.send), len(b.send))
	}
}

// TestRunsForConn confirms a conn's subscribed runs are reported back —
// the set the SSE handler scopes Last-Event-Id replay to.
func TestRunsForConn(t *testing.T) {
	reg := newClientRegistry()
	reg.subscribe("run_1", "conn-a")
	reg.subscribe("run_2", "conn-a")
	reg.subscribe("run_3", "conn-b")

	got := reg.runsForConn("conn-a")
	slices.Sort(got)
	if !slices.Equal(got, []string{"run_1", "run_2"}) {
		t.Fatalf("runsForConn(conn-a) = %v, want [run_1 run_2]", got)
	}
	if got := reg.runsForConn("conn-x"); len(got) != 0 {
		t.Fatalf("runsForConn(unknown) = %v, want empty", got)
	}
}

// TestReconnectReplacesConn confirms a reconnect under the same connID
// swaps in the new connection (one stream per conn id, TRANSPORT §7) and
// closes the old one, while the subscription survives so the new socket
// keeps receiving the run's events.
func TestReconnectReplacesConn(t *testing.T) {
	reg := newClientRegistry()
	old := newConn()
	reg.register("conn-a", old)
	reg.subscribe("run_1", "conn-a")

	fresh := newConn()
	reg.register("conn-a", fresh) // reconnect under same id

	select {
	case <-old.done:
		// expected: old connection was closed
	default:
		t.Fatal("old conn should be closed on reconnect")
	}

	reg.routeToRun("run_1", testNotif())
	if len(fresh.send) != 1 {
		t.Fatalf("fresh conn got %d, want 1 (subscription must survive reconnect)", len(fresh.send))
	}
}
