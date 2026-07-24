package runs

import (
	"iter"
	"sync"
)

// liveHeadroom bounds queued live-only events per subscriber. Durable and
// terminal events are never subject to this budget.
const liveHeadroom = 256

// Journal is the per-run event fan-out + durable replay buffer. Each subscriber
// drains a cond-guarded queue on the consumer's own goroutine; Append only
// enqueues, so a slow consumer cannot stall the run. Durable events remain
// ordered and lossless while excess live-only deltas are coalesced by dropping
// them at the subscriber boundary.
type Journal struct {
	mu        sync.Mutex
	durable   []Event
	subs      map[int]*journalSubscriber
	nextSubID int
	closed    bool
}

// NewJournal builds an empty Journal for application run events.
func NewJournal() *Journal {
	return &Journal{subs: map[int]*journalSubscriber{}}
}

// Append retains a durable event and enqueues the event for every live
// subscriber. Durable and terminal events are lossless; only excess live-only
// events can be dropped. Enqueue never blocks on consumers.
func (j *Journal) Append(ev Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	if ev.Durable() {
		j.durable = append(j.durable, ev)
	}
	for _, subscriber := range j.subs {
		subscriber.enqueue(ev)
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

// Subscribe returns the durable backlog after fromCursor followed by live events
// as one ordered sequence, plus an idempotent cancel. Subscribe and Append
// serialize on the Journal lock, so replay and the first live event form one
// ordered stream.
//
// Stopping an active range detaches the subscriber automatically. cancel remains
// necessary when the consumer must interrupt a range blocked waiting for its
// next event, so callers should also wire it to the consumer's lifetime.
func (j *Journal) Subscribe(fromCursor string) (iter.Seq[Event], func()) {
	j.mu.Lock()
	replay := make([]Event, 0, len(j.durable))
	for _, ev := range j.durable {
		if fromCursor == "" || ev.Cursor() > fromCursor {
			replay = append(replay, ev)
		}
	}
	if j.closed {
		j.mu.Unlock()
		subscriber := newJournalSubscriber(replay)
		subscriber.finish() // no live tail: drain the backlog, then end
		return subscriber.events(), func() {}
	}

	subscriber := newJournalSubscriber(replay)
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
	return func(yield func(Event) bool) {
		defer cancel()
		events(yield)
	}, cancel
}

type journalSubscriber struct {
	mu         sync.Mutex
	ready      *sync.Cond
	queue      []Event
	queuedLive int
	finishing  bool
	aborted    bool
}

func newJournalSubscriber(replay []Event) *journalSubscriber {
	subscriber := &journalSubscriber{queue: replay}
	subscriber.ready = sync.NewCond(&subscriber.mu)
	return subscriber
}

func (s *journalSubscriber) enqueue(ev Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finishing || s.aborted {
		return
	}
	if !ev.Durable() && !ev.Terminal() {
		if s.queuedLive >= liveHeadroom {
			return
		}
		s.queuedLive++
	}
	s.queue = append(s.queue, ev)
	s.ready.Signal()
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
	for len(s.queue) == 0 && !s.finishing && !s.aborted {
		s.ready.Wait()
	}
	if s.aborted || (len(s.queue) == 0 && s.finishing) {
		return Event{}, false
	}
	ev := s.queue[0]
	s.queue[0] = Event{}
	s.queue = s.queue[1:]
	if !ev.Durable() && !ev.Terminal() {
		s.queuedLive--
	}
	return ev, true
}
