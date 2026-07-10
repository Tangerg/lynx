package runs

import (
	"sync"
	"time"
)

// Streamable is the minimum the [Journal] needs to buffer, fan out, and replay a
// run event — nothing wire-specific. The concrete event type belongs to the
// caller (delivery supplies its protocol RunEvent; the application supplies its
// own [Event]); the Journal never interprets the payload, which keeps run-stream
// ownership in the application while the wire shape stays in delivery.
type Streamable interface {
	// Durable reports whether the event is retained for replay. Live-only events
	// are fanned out to current subscribers but never buffered, so a
	// reconnecting subscriber never re-receives them.
	Durable() bool
	// Terminal reports whether the event ends the run's stream. A dropped
	// terminal strands the client, so the Journal delivers it with a bounded
	// blocking send and expects Close to follow.
	Terminal() bool
	// Cursor is the event's monotonic stream position. Replay yields only events
	// strictly after a requested cursor, compared lexically — so cursors must be
	// fixed-width and lexically ordered (the wire's zero-padded ids are, which is
	// what lets the Journal stay ignorant of their format).
	Cursor() string
}

const (
	// liveHeadroom is the spare capacity a subscriber channel keeps beyond its
	// replay backlog, to absorb live events while the consumer drains. A
	// subscriber that overflows drops live events and must reconnect.
	liveHeadroom = 256
	// terminalSendTimeout bounds the total time [Journal.Append] blocks
	// delivering a terminal event to (possibly backpressured) subscribers.
	terminalSendTimeout = 2 * time.Second
)

// Journal is the per-run event fan-out + durable replay buffer that owns a run's
// stream for its whole lifetime, independent of any client connection: the run's
// pump Appends every event, and each streaming call Subscribes — replaying the
// durable backlog after its cursor, then tailing live — so a dropped connection
// never stalls the run and a re-subscribe resumes cleanly.
//
// Only durable events are retained for replay; live-only events reach current
// subscribers but are not buffered. The pump calls Close on the terminal event;
// the Journal doesn't otherwise interpret events.
type Journal[E Streamable] struct {
	mu        sync.Mutex
	durable   []E
	subs      map[int]chan E
	nextSubID int
	closed    bool
}

// NewJournal builds an empty Journal for events of type E.
func NewJournal[E Streamable]() *Journal[E] {
	return &Journal[E]{subs: map[int]chan E{}}
}

// Append fans ev out to every live subscriber and, when ev is durable, retains
// it for replay. Per-subscriber delivery is non-blocking — a full channel drops
// the event so one slow consumer can't stall the run (it recovers by
// reconnecting) — EXCEPT a terminal event, delivered with a bounded blocking
// send so it can't be the one event a backpressured consumer loses. The blocking
// send holds mu, which is safe: Close and a subscriber's cancel also take mu, so
// a channel can't be closed under us (no send-on-closed panic); a draining
// consumer frees a slot immediately, and only a vanished one waits out the
// timeout.
func (j *Journal[E]) Append(ev E) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	if ev.Durable() {
		j.durable = append(j.durable, ev)
	}
	if ev.Terminal() {
		deliverTerminal(j.subs, ev, terminalSendTimeout)
		return
	}
	for _, ch := range j.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// deliverTerminal gives every healthy subscriber an immediate chance before
// waiting on backpressured ones under one shared deadline. Call with the
// Journal's mu held, which keeps subscriber channels open for the duration.
func deliverTerminal[E Streamable](subs map[int]chan E, ev E, budget time.Duration) {
	blocked := make([]chan E, 0, len(subs))
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
			blocked = append(blocked, ch)
		}
	}
	if len(blocked) == 0 {
		return
	}
	timer := time.NewTimer(budget)
	defer timer.Stop()
	for _, ch := range blocked {
		select {
		case ch <- ev:
		case <-timer.C:
			return
		}
	}
}

// Close ends the run's stream: every subscriber channel is closed (the
// consumer's end-of-stream signal). Idempotent. Subscribers that attach after
// Close get the durable replay followed by a closed channel.
func (j *Journal[E]) Close() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	j.closed = true
	for id, ch := range j.subs {
		close(ch)
		delete(j.subs, id)
	}
}

// Subscribe returns a channel delivering the durable backlog after fromCursor
// (empty = from the start) followed by live events, plus a cancel func to
// detach. When the Journal is already closed the channel carries the replay then
// closes. The channel is sized for the replay plus live headroom so the replay
// enqueue never blocks.
func (j *Journal[E]) Subscribe(fromCursor string) (<-chan E, func()) {
	j.mu.Lock()
	defer j.mu.Unlock()

	replay := make([]E, 0, len(j.durable))
	for _, ev := range j.durable {
		if fromCursor == "" || ev.Cursor() > fromCursor {
			replay = append(replay, ev)
		}
	}
	ch := make(chan E, len(replay)+liveHeadroom)
	for _, ev := range replay {
		ch <- ev
	}

	if j.closed {
		close(ch)
		return ch, func() {}
	}

	id := j.nextSubID
	j.nextSubID++
	j.subs[id] = ch
	cancel := func() {
		j.mu.Lock()
		defer j.mu.Unlock()
		if c, ok := j.subs[id]; ok {
			delete(j.subs, id)
			close(c)
		}
	}
	return ch, cancel
}
