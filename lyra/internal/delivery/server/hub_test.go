package server

import (
	"fmt"
	"testing"

	"github.com/Tangerg/lynx/lyra/internal/delivery/protocol"
)

// ev builds a RunEvent with a padded eventId and the given durability —
// durability now derives from the event type (StreamEvent.IsDurable), so we
// pick a durable type (item.completed) or an ephemeral one (item.delta).
func ev(seq int, durable bool) protocol.RunEvent {
	t := protocol.StreamItemDelta // ephemeral
	if durable {
		t = protocol.StreamItemCompleted
	}
	return protocol.RunEvent{
		EventID: fmt.Sprintf("%s%011d", protocol.IDPrefixEvent, seq),
		Event:   protocol.StreamEvent{Type: t},
	}
}

func drain(ch <-chan protocol.RunEvent) []string {
	var ids []string
	for e := range ch {
		ids = append(ids, e.EventID)
	}
	return ids
}

// TestRunHub_ReplayThenLive: a subscriber gets the durable backlog first,
// then live events, in order.
func TestRunHub_ReplayThenLive(t *testing.T) {
	h := newRunHub()
	h.Append(ev(1, true))
	h.Append(ev(2, true))

	ch, cancel := h.Subscribe("")
	defer cancel()

	if got := <-ch; got.EventID != ev(1, true).EventID {
		t.Fatalf("first = %s, want evt 1", got.EventID)
	}
	if got := <-ch; got.EventID != ev(2, true).EventID {
		t.Fatalf("second = %s, want evt 2", got.EventID)
	}

	h.Append(ev(3, true)) // live
	if got := <-ch; got.EventID != ev(3, true).EventID {
		t.Fatalf("live = %s, want evt 3", got.EventID)
	}
}

// TestRunHub_SubscribeFromEventID: replay only the backlog strictly after
// the supplied Last-Event-Id.
func TestRunHub_SubscribeFromEventID(t *testing.T) {
	h := newRunHub()
	for i := 1; i <= 3; i++ {
		h.Append(ev(i, true))
	}
	ch, cancel := h.Subscribe(ev(2, true).EventID)
	defer cancel()
	h.Close() // no live; just drain replay

	got := drain(ch)
	if len(got) != 1 || got[0] != ev(3, true).EventID {
		t.Fatalf("replay = %v, want [evt 3]", got)
	}
}

// TestRunHub_EphemeralNotReplayed: ephemeral (durable=false) events reach
// live subscribers but are never in a later subscriber's replay (§9.3).
func TestRunHub_EphemeralNotReplayed(t *testing.T) {
	h := newRunHub()

	live, cancel := h.Subscribe("")
	defer cancel()
	h.Append(ev(1, true))
	h.Append(ev(2, false)) // ephemeral
	if (<-live).EventID != ev(1, true).EventID {
		t.Fatal("live missing durable evt 1")
	}
	if (<-live).EventID != ev(2, false).EventID {
		t.Fatal("live missing ephemeral evt 2")
	}

	// A fresh subscriber replays durable only.
	late, cancel2 := h.Subscribe("")
	defer cancel2()
	h.Close()
	if got := drain(late); len(got) != 1 || got[0] != ev(1, true).EventID {
		t.Fatalf("late replay = %v, want [evt 1] (no ephemeral)", got)
	}
}

// TestRunHub_FanOutN: every subscriber receives each live event.
func TestRunHub_FanOutN(t *testing.T) {
	h := newRunHub()
	a, ca := h.Subscribe("")
	defer ca()
	b, cb := h.Subscribe("")
	defer cb()

	h.Append(ev(1, true))
	if (<-a).EventID != ev(1, true).EventID || (<-b).EventID != ev(1, true).EventID {
		t.Fatal("both subscribers must receive evt 1")
	}
}

// TestRunHub_CloseEndsStream: Close closes every subscriber channel, and
// a post-close Subscribe replays the backlog then closes.
func TestRunHub_CloseEndsStream(t *testing.T) {
	h := newRunHub()
	h.Append(ev(1, true))
	ch, cancel := h.Subscribe("")
	defer cancel()
	<-ch // drain replay

	h.Close()
	if _, ok := <-ch; ok {
		t.Fatal("channel must close on hub Close")
	}

	post, _ := h.Subscribe("")
	if got := drain(post); len(got) != 1 || got[0] != ev(1, true).EventID {
		t.Fatalf("post-close replay = %v, want [evt 1] then closed", got)
	}
}

// TestRunHub_CancelDetaches: after cancel, the subscriber stops receiving
// and a later Close doesn't double-close its channel.
func TestRunHub_CancelDetaches(t *testing.T) {
	h := newRunHub()
	ch, cancel := h.Subscribe("")
	cancel()
	if _, ok := <-ch; ok {
		t.Fatal("cancel must close the channel")
	}
	h.Append(ev(1, true)) // must not panic (sub gone)
	h.Close()             // must not double-close
}
