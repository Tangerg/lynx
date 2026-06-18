package server

import (
	"strconv"
	"strings"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// liveHeadroom is the spare capacity a subscriber channel keeps beyond
// its initial replay backlog, to absorb live events while the consumer
// drains. A subscriber that overflows it drops live events and must
// reconnect (runs.subscribe replays the durable backlog; items.list is
// the ultimate backstop, API.md §5.2).
const liveHeadroom = 256

// runHub is the per-run event fan-out + durable replay buffer behind
// streamable-HTTP delivery (TRANSPORT §6.4 / §9.2). One hub per root run,
// independent of any client connection: the run's pump appends every
// RunEvent here for the run's lifetime, and each streaming call
// (runs.start / runs.subscribe) Subscribes — replaying the durable
// backlog after its Last-Event-Id, then tailing live — so a dropped
// connection never stalls the run and a re-subscribe resumes cleanly.
//
// Only durable events are retained for replay (API.md §9.3: item.delta /
// state.delta are never replayed); ephemeral events reach live
// subscribers but are not buffered. Durability is derived from the event
// itself (StreamEvent.IsDurable, the §5.2 SSOT) — no per-frame bool. The
// pump calls Close on the terminal run.finished; the hub doesn't otherwise
// interpret events.
type runHub struct {
	mu        sync.Mutex
	durable   []protocol.RunEvent // durable backlog, retained for the hub's life
	subs      map[int]chan protocol.RunEvent
	nextSubID int
	closed    bool
}

func newRunHub() *runHub {
	return &runHub{subs: map[int]chan protocol.RunEvent{}}
}

// Append fans ev out to every live subscriber and, when ev is durable,
// retains it for replay. Per-subscriber delivery is non-blocking: a full
// channel drops the event so one slow consumer can't stall the run or the
// other subscribers — the consumer recovers by reconnecting.
func (h *runHub) Append(ev protocol.RunEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	if ev.Event.IsDurable() {
		h.durable = append(h.durable, ev)
	}
	for _, ch := range h.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// Close ends the run's stream: every subscriber channel is closed (the
// consumer's end-of-stream signal). Idempotent. Subscribers that attach
// after Close get the durable replay followed by a closed channel.
func (h *runHub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for id, ch := range h.subs {
		close(ch)
		delete(h.subs, id)
	}
}

// Subscribe returns a channel delivering the durable backlog after
// fromEventID (empty = from the start) followed by live events, plus a
// cancel func to detach. When the hub is already closed the channel
// carries the replay then closes. The channel is sized for the replay
// plus live headroom so the replay enqueue never blocks.
func (h *runHub) Subscribe(fromEventID string) (<-chan protocol.RunEvent, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()

	replay := make([]protocol.RunEvent, 0, len(h.durable))
	for _, ev := range h.durable {
		if fromEventID == "" || compareEventID(ev.EventID, fromEventID) > 0 {
			replay = append(replay, ev)
		}
	}
	ch := make(chan protocol.RunEvent, len(replay)+liveHeadroom)
	for _, ev := range replay {
		ch <- ev
	}

	if h.closed {
		close(ch)
		return ch, func() {}
	}

	id := h.nextSubID
	h.nextSubID++
	h.subs[id] = ch
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if c, ok := h.subs[id]; ok {
			delete(h.subs, id)
			close(c)
		}
	}
	return ch, cancel
}

// compareEventID compares two evt_<zero-padded-decimal> ids numerically
// (TRANSPORT §9.1). The fixed-width padding makes lexical order agree
// with numeric, but parsing is exact; a malformed id falls back to
// lexical compare rather than silently comparing equal.
func compareEventID(a, b string) int {
	an, aerr := strconv.ParseUint(strings.TrimPrefix(a, protocol.IDPrefixEvent), 10, 64)
	bn, berr := strconv.ParseUint(strings.TrimPrefix(b, protocol.IDPrefixEvent), 10, 64)
	if aerr == nil && berr == nil {
		switch {
		case an < bn:
			return -1
		case an > bn:
			return 1
		default:
			return 0
		}
	}
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
