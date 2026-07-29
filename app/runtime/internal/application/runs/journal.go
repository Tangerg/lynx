package runs

import (
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"sync"

	"github.com/Tangerg/lynx/app/runtime/internal/component/replaycursor"
)

// liveHeadroom bounds queued live-only events per subscriber. Authoritative
// events are never subject to this budget — they are bounded by [Retention]
// instead, and a subscriber that exceeds THAT is disconnected rather than
// quietly served an incomplete stream.
const liveHeadroom = 256

// Retention is the replay window one live segment keeps for reconnecting
// subscribers. Replay is not a second durable event store: history belongs to
// the transcript reads, and this window only covers the gap a reconnect opens.
//
// Both budgets are enforced, and both are needed. The count bounds how far back
// a cursor can reach; the byte budget bounds what that costs, because a single
// segment can carry a handful of events that each hold a multi-megabyte tool
// result or image. Whichever is reached first evicts from the oldest event
// forward.
//
// The values are published through discovery so a client can choose replay or a
// cold read knowingly. They are values rather than protocol: tuning them is not
// a protocol change, but changing what the window BELONGS to would be.
type Retention struct {
	MaxEvents int
	MaxBytes  int
}

// DefaultRetention is the window this runtime enforces and advertises.
var DefaultRetention = Retention{MaxEvents: 2048, MaxBytes: 16 << 20}

// Replay refusals. They are two rather than one because the client's next move
// differs: an invalid cursor is a cursor that never addressed this stream, so
// carrying it forward would keep failing, while an unavailable one addressed a
// stream this process can no longer produce, so the remedy is to rebuild from
// the durable reads.
var (
	// ErrReplayCursorInvalid: the cursor cannot be read, names another stream, or
	// names a position this stream never reached. All three mean the client is
	// holding something it should not act on.
	ErrReplayCursorInvalid = errors.New("runs: replay cursor does not address this stream")
	// ErrReplayUnavailable: the cursor was legitimate, and what it pointed at is
	// gone — a previous process's stream, or a position the window has evicted.
	ErrReplayUnavailable = errors.New("runs: replay window no longer reaches this cursor")
)

// streamScope is what one Journal's positions are minted for: the process that
// owns the buffer, and the run and segment whose stream it is. A cursor carries
// the same three, so one from another process or another segment is refused
// instead of resolving against a stream that never issued it.
type streamScope struct {
	Epoch     string
	RunID     string
	SegmentID string
}

// subscription is one attached consumer of a Journal.
type subscription struct {
	Events iter.Seq[Event]
	// Cancel detaches idempotently. Stopping an active range detaches too;
	// Cancel remains necessary to interrupt a range blocked waiting for its next
	// event, so callers wire it to the consumer's lifetime as well.
	Cancel func()
	// HeadCursor is the last position already published when this subscription
	// attached, or "" when the stream had published nothing yet. A client stores
	// it verbatim and hands it back on reconnect; it is not a watermark and
	// nothing may order or parse it.
	HeadCursor string
}

// Journal is one live segment's event stream: the position authority, the replay
// window, and the fan-out. Each subscriber drains a cond-guarded queue on its own
// goroutine and Append only enqueues, so a slow consumer cannot stall the run.
//
// It is created per segment and released with it, which is what makes the
// advertised replay scope true: a resumed run gets a new Journal, so a cursor
// from the previous segment is refused by scope rather than silently continued
// against a different stream.
type Journal struct {
	mu        sync.Mutex
	scope     streamScope
	retention Retention
	// head is the last sequence assigned. Zero means nothing has been published,
	// which is why a cursor's sequence starts at 1.
	head uint64
	// retained is the replay window: authoritative events, oldest first, each
	// carrying what it was charged.
	retained      []chargedEvent
	retainedBytes int
	// evictedThrough is the highest sequence dropped from the window. A cursor
	// before it has lost at least one authoritative event, which is the difference
	// between "replay this" and "you must recover from the reads".
	evictedThrough uint64
	subs           map[int]*journalSubscriber
	nextSubID      int
	closed         bool
}

// NewJournal builds the stream for one segment. scope binds every position it
// mints; retention bounds both the replay window and any one subscriber's
// authoritative backlog.
func newJournal(scope streamScope, retention Retention) *Journal {
	return &Journal{scope: scope, retention: retention, subs: map[int]*journalSubscriber{}}
}

// Append assigns ev its position in this stream, retains it if it is
// authoritative, and enqueues it for every live subscriber.
//
// The position is assigned HERE, under the same lock as the fan-out, so
// sequence order, publication order and replay order are one order rather than
// three that agree by convention.
func (j *Journal) Append(ev Event) {
	// Charging serializes the payload — milliseconds for a multi-megabyte tool
	// result — and it needs nothing from this Journal. So it happens before the
	// lock: inside, it would hold the run's ONLY stream for the length of a
	// marshal, and position assignment and every subscriber's enqueue wait behind
	// that lock.
	size := 0
	if ev.Durable() {
		size = retainedSize(ev)
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	j.head++
	ev.Sequence = j.head
	ev.Cursor = replaycursor.Encode(replaycursor.Position{
		Epoch: j.scope.Epoch, RunID: j.scope.RunID,
		SegmentID: j.scope.SegmentID, Sequence: j.head,
	})
	if ev.Durable() {
		j.retained = append(j.retained, chargedEvent{event: ev, bytes: size})
		j.retainedBytes += size
		j.evictLocked()
	}
	for _, subscriber := range j.subs {
		subscriber.enqueue(ev, size)
	}
}

// evictLocked drops the oldest authoritative events until the window fits both
// budgets, recording how far the eviction reached so a cursor behind it can be
// told the difference between "nothing new" and "you missed something".
func (j *Journal) evictLocked() {
	for len(j.retained) > 0 &&
		(len(j.retained) > j.retention.MaxEvents || j.retainedBytes > j.retention.MaxBytes) {
		oldest := j.retained[0]
		j.retained[0] = chargedEvent{}
		j.retained = j.retained[1:]
		j.retainedBytes -= oldest.bytes
		j.evictedThrough = oldest.event.Sequence
	}
}

// Close ends the run's stream. Each subscriber drains its already-enqueued
// events in order and then its sequence returns. Close does not wait, which lets
// a stream opened by a fast run return to its caller before that caller starts
// draining.
func (j *Journal) Close() {
	j.mu.Lock()
	if j.closed {
		j.mu.Unlock()
		return
	}
	j.closed = true
	subscribers := make([]*journalSubscriber, 0, len(j.subs))
	for id, subscriber := range j.subs {
		subscribers = append(subscribers, subscriber)
		delete(j.subs, id)
	}
	j.mu.Unlock()

	for _, subscriber := range subscribers {
		subscriber.finish()
	}
}

// Tail attaches from the current head: the subscriber receives only what is
// appended after this call, and HeadCursor names the position it starts after.
//
// This is what "no cursor" means. Replaying the whole segment instead would hand
// back a stream whose beginning the client must reconcile against the transcript
// reads anyway — and doing both is how an event is folded twice. Capturing the
// head and attaching happen under one lock, so nothing is published in between:
// a client can read the durable state afterwards and fold this stream on top
// without a gap.
func (j *Journal) Tail() subscription {
	j.mu.Lock()
	head := j.headCursorLocked()
	return j.attachLocked(nil, head)
}

// attach subscribes from the position token names, or from the current head when
// token is empty.
//
// An empty cursor is tail-only. It is the one place that decision is made,
// because the alternative reading — "no cursor means replay everything" — is a
// stream whose start the client must reconcile against the transcript reads it
// is already making, and folding both is how one event is applied twice.
func (j *Journal) attach(token string) (subscription, error) {
	if token == "" {
		return j.Tail(), nil
	}
	return j.Replay(token)
}

// Replay attaches after the position token names, delivering the retained events
// beyond it before the live tail.
//
// It reports [ErrReplayCursorInvalid] when the token cannot be read, was minted
// for another stream, or names a position past this stream's head, and
// [ErrReplayUnavailable] when it came from a previous process or has been
// evicted from the window.
func (j *Journal) Replay(token string) (subscription, error) {
	from, err := replaycursor.Decode(token)
	if err != nil {
		return subscription{}, fmt.Errorf("%w: %w", ErrReplayCursorInvalid, err)
	}
	j.mu.Lock()
	// Epoch before scope: a cursor from a previous process cannot be expected to
	// name a run this process knows, so blaming its scope would report the client's
	// second problem instead of its first.
	if from.Epoch != j.scope.Epoch {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor was minted by another runtime process", ErrReplayUnavailable)
	}
	if from.RunID != j.scope.RunID || from.SegmentID != j.scope.SegmentID {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor belongs to run %s segment %s",
			ErrReplayCursorInvalid, from.RunID, from.SegmentID)
	}
	if from.Sequence > j.head {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor is ahead of the stream", ErrReplayCursorInvalid)
	}
	if from.Sequence < j.evictedThrough {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor precedes the retained window", ErrReplayUnavailable)
	}
	backlog := make([]chargedEvent, 0, len(j.retained))
	for _, retained := range j.retained {
		if retained.event.Sequence > from.Sequence {
			backlog = append(backlog, retained)
		}
	}
	return j.attachLocked(backlog, j.headCursorLocked()), nil
}

// headCursorLocked is the cursor for the last published position, or "" when the
// stream has published nothing.
func (j *Journal) headCursorLocked() string {
	if j.head == 0 {
		return ""
	}
	return replaycursor.Encode(replaycursor.Position{
		Epoch: j.scope.Epoch, RunID: j.scope.RunID,
		SegmentID: j.scope.SegmentID, Sequence: j.head,
	})
}

// attachLocked registers a subscriber primed with backlog and releases the lock.
// Attaching under the same lock as Append is what makes replay and the first live
// event one ordered stream.
func (j *Journal) attachLocked(backlog []chargedEvent, head string) subscription {
	if j.closed {
		j.mu.Unlock()
		subscriber := newJournalSubscriber(backlog, j.retention)
		subscriber.finish() // no live tail: drain the backlog, then end
		return subscription{Events: subscriber.events(), Cancel: func() {}, HeadCursor: head}
	}

	subscriber := newJournalSubscriber(backlog, j.retention)
	id := j.nextSubID
	j.nextSubID++
	j.subs[id] = subscriber
	j.mu.Unlock()

	var cancelOnce sync.Once
	cancel := func() {
		cancelOnce.Do(func() {
			j.mu.Lock()
			delete(j.subs, id)
			j.mu.Unlock()
			subscriber.abort()
		})
	}
	events := subscriber.events()
	return subscription{
		Events: func(yield func(Event) bool) {
			defer cancel()
			events(yield)
		},
		Cancel:     cancel,
		HeadCursor: head,
	}
}

// retainedSize is what the replay window charges for one event: the length of its
// serialized payload.
//
// The window exists to bound memory, and the dominant term in every
// authoritative event is the content it carries — item text, tool output, base64
// image data — which serializing measures directly. It is measured rather than
// estimated per event type because an estimate returns zero for whatever field
// nobody remembered to add, and a budget that silently stops counting is a budget
// that stops bounding. A payload that cannot be serialized measures zero and is
// still bounded by the event count.
//
// Called exactly ONCE per event, at publication. It used to be called again on
// eviction and again for every backlog event on every replay attach — a
// reconnect re-serialized the whole window to recompute numbers the window had
// already charged, which measured at 4.67 MB per attach on a 4 MiB backlog.
// [chargedEvent] carries the answer instead.
func retainedSize(ev Event) int {
	payload, _ := json.Marshal(ev.Payload)
	return len(payload)
}

// chargedEvent is an event together with what the byte budget charged for it.
// The window, its eviction and every replaying subscriber read that one number
// rather than re-deriving it from the payload.
type chargedEvent struct {
	event Event
	bytes int
}

type journalSubscriber struct {
	mu        sync.Mutex
	ready     *sync.Cond
	retention Retention
	queue     []chargedEvent
	head      int
	// queuedLive counts ephemeral events awaiting delivery; queuedDurable and
	// queuedBytes count the authoritative backlog, which cannot be dropped and
	// therefore ends the subscription instead when it exceeds the window.
	queuedLive    int
	queuedDurable int
	queuedBytes   int
	finishing     bool
	aborted       bool
}

// newJournalSubscriber primes a subscriber with a replay backlog. The backlog
// arrives already charged — dequeuing then releases exactly what the window
// charged at publication, and an attach costs a slice copy rather than a
// re-serialization of everything the window holds.
func newJournalSubscriber(backlog []chargedEvent, retention Retention) *journalSubscriber {
	subscriber := &journalSubscriber{
		retention: retention,
		queue:     append(make([]chargedEvent, 0, len(backlog)), backlog...),
	}
	subscriber.ready = sync.NewCond(&subscriber.mu)
	subscriber.queuedDurable = len(backlog)
	for _, charged := range backlog {
		subscriber.queuedBytes += charged.bytes
	}
	return subscriber
}

// enqueue queues one event, or drops it when it is ephemeral and the consumer is
// behind. size is what the journal charged for it, zero for ephemeral events.
//
// An authoritative event is never dropped: a stream missing one is a stream the
// client would fold into a wrong state without being able to tell. So a consumer
// that lets the authoritative backlog reach the retention window is disconnected
// instead, and reads the abnormal end of stream as "reconnect and recover".
func (s *journalSubscriber) enqueue(ev Event, size int) {
	s.mu.Lock()
	if s.finishing || s.aborted {
		s.mu.Unlock()
		return
	}
	if !ev.Durable() && !ev.Terminal() {
		if s.queuedLive >= liveHeadroom {
			s.mu.Unlock()
			return
		}
		s.queuedLive++
	} else {
		if s.queuedDurable >= s.retention.MaxEvents || s.queuedBytes+size > s.retention.MaxBytes {
			s.mu.Unlock()
			s.abort()
			return
		}
		s.queuedDurable++
		s.queuedBytes += size
	}
	if s.head > 0 && len(s.queue) == cap(s.queue) {
		remaining := copy(s.queue, s.queue[s.head:])
		clear(s.queue[remaining:])
		s.queue = s.queue[:remaining]
		s.head = 0
	}
	s.queue = append(s.queue, chargedEvent{event: ev, bytes: size})
	s.ready.Signal()
	s.mu.Unlock()
}

// finish marks the stream complete: the subscriber drains its remaining queue in
// order, then its sequence ends.
func (s *journalSubscriber) finish() {
	s.mu.Lock()
	s.finishing = true
	s.ready.Broadcast()
	s.mu.Unlock()
}

// abort ends the subscription immediately, abandoning any queued events, and
// wakes a consumer blocked waiting for the next event.
func (s *journalSubscriber) abort() {
	s.mu.Lock()
	s.aborted = true
	clear(s.queue[s.head:])
	s.queue = nil
	s.head = 0
	s.queuedLive = 0
	s.queuedDurable = 0
	s.queuedBytes = 0
	s.ready.Broadcast()
	s.mu.Unlock()
}

// events yields queued events until the subscription drains to a finish or is
// aborted. It runs on the consumer's goroutine; next blocks in the cond, so
// finish or abort must wake a consumer that is waiting between events.
func (s *journalSubscriber) events() iter.Seq[Event] {
	return func(yield func(Event) bool) {
		for {
			ev, ok := s.next()
			if !ok {
				return
			}
			if !yield(ev) {
				return
			}
		}
	}
}

func (s *journalSubscriber) next() (Event, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for s.head == len(s.queue) && !s.finishing && !s.aborted {
		s.ready.Wait()
	}
	if s.aborted || (s.head == len(s.queue) && s.finishing) {
		return Event{}, false
	}
	queued := s.queue[s.head]
	s.queue[s.head] = chargedEvent{}
	s.head++
	if s.head == len(s.queue) {
		// Reuse the routine live-event buffer, but do not retain a durable burst.
		if cap(s.queue) > liveHeadroom {
			s.queue = nil
		} else {
			s.queue = s.queue[:0]
		}
		s.head = 0
	}
	if !queued.event.Durable() && !queued.event.Terminal() {
		s.queuedLive--
	} else {
		s.queuedDurable--
		s.queuedBytes -= queued.bytes
	}
	return queued.event, true
}
