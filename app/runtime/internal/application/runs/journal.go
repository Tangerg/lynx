package runs

import (
	"sync"
	"time"
)

// Streamable is the minimum the [Journal] needs to buffer, fan out, and replay a
// run event — nothing wire-specific. The concrete event type belongs to the
// caller; the Journal never interprets its payload.
type Streamable interface {
	// Durable reports whether the event is retained for replay. A live subscriber
	// also receives every durable event in order, regardless of backpressure.
	Durable() bool
	// Terminal reports whether the event ends the run's stream. Terminal events
	// are never dropped, even if a concrete event family marks one non-durable.
	Terminal() bool
	// Cursor is the event's monotonic stream position. Replay yields only events
	// strictly after a requested cursor, compared lexically — cursors must therefore
	// be fixed-width and lexically ordered.
	Cursor() string
}

const (
	// liveHeadroom bounds queued live-only events per subscriber. Durable and
	// terminal events are never subject to this budget.
	liveHeadroom = 256
	// terminalSendTimeout bounds how long a finished Journal lets an abandoned
	// subscriber drain its final queue before aborting that subscription.
	terminalSendTimeout = 2 * time.Second
)

// Journal is the per-run event fan-out + durable replay buffer. Each subscriber
// owns a small delivery pump: Append only enqueues, so a slow consumer cannot
// stall the run; durable events remain ordered and lossless while excess
// live-only deltas are coalesced by dropping them at the subscriber boundary.
type Journal[E Streamable] struct {
	mu        sync.Mutex
	durable   []E
	subs      map[int]*journalSubscriber[E]
	nextSubID int
	closed    bool
}

// NewJournal builds an empty Journal for events of type E.
func NewJournal[E Streamable]() *Journal[E] {
	return &Journal[E]{subs: map[int]*journalSubscriber[E]{}}
}

// Append retains a durable event and enqueues the event for every live
// subscriber. Durable and terminal events are lossless; only excess live-only
// events can be dropped. Per-subscriber delivery pumps keep Append non-blocking
// with respect to consumers.
func (j *Journal[E]) Append(ev E) {
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

// Close ends the run's stream. Subscribers drain their already-enqueued events
// in order and then close; an abandoned consumer is aborted after one bounded
// shared-duration window. Close itself does not wait, which lets a stream opened
// by a fast run return to its caller before that caller starts draining.
func (j *Journal[E]) Close() {
	j.mu.Lock()
	if j.closed {
		j.mu.Unlock()
		return
	}
	j.closed = true
	subscribers := make([]*journalSubscriber[E], 0, len(j.subs))
	for id, subscriber := range j.subs {
		subscribers = append(subscribers, subscriber)
		delete(j.subs, id)
	}
	j.mu.Unlock()

	for _, subscriber := range subscribers {
		subscriber.finish(terminalSendTimeout)
	}
}

// Subscribe returns the durable backlog after fromCursor followed by live
// events, plus an idempotent cancel function. Subscribe and Append serialize on
// the Journal lock, so replay and the first live event form one ordered stream.
func (j *Journal[E]) Subscribe(fromCursor string) (<-chan E, func()) {
	j.mu.Lock()
	replay := make([]E, 0, len(j.durable))
	for _, ev := range j.durable {
		if fromCursor == "" || ev.Cursor() > fromCursor {
			replay = append(replay, ev)
		}
	}
	if j.closed {
		j.mu.Unlock()
		out := make(chan E, len(replay))
		for _, ev := range replay {
			out <- ev
		}
		close(out)
		return out, func() {}
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
			<-subscriber.stopped
		})
	}
	return subscriber.out, cancel
}

type journalSubscriber[E Streamable] struct {
	mu         sync.Mutex
	ready      *sync.Cond
	queue      []E
	queuedLive int
	finishing  bool
	aborted    bool
	abortOnce  sync.Once
	abortCh    chan struct{}
	out        chan E
	stopped    chan struct{}
	timer      *time.Timer
}

func newJournalSubscriber[E Streamable](replay []E) *journalSubscriber[E] {
	subscriber := &journalSubscriber[E]{
		queue:   replay,
		abortCh: make(chan struct{}),
		out:     make(chan E, liveHeadroom),
		stopped: make(chan struct{}),
	}
	subscriber.ready = sync.NewCond(&subscriber.mu)
	go subscriber.run()
	return subscriber
}

func (s *journalSubscriber[E]) enqueue(ev E) {
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

func (s *journalSubscriber[E]) finish(budget time.Duration) {
	s.mu.Lock()
	if s.finishing || s.aborted {
		s.mu.Unlock()
		return
	}
	s.finishing = true
	s.timer = time.AfterFunc(budget, s.abort)
	s.ready.Broadcast()
	s.mu.Unlock()
}

func (s *journalSubscriber[E]) abort() {
	s.abortOnce.Do(func() {
		s.mu.Lock()
		s.aborted = true
		close(s.abortCh)
		s.ready.Broadcast()
		s.mu.Unlock()
	})
}

func (s *journalSubscriber[E]) run() {
	defer func() {
		s.mu.Lock()
		if s.timer != nil {
			s.timer.Stop()
		}
		s.mu.Unlock()
		close(s.out)
		close(s.stopped)
	}()
	for {
		ev, ok := s.next()
		if !ok {
			return
		}
		select {
		case s.out <- ev:
		case <-s.abortCh:
			return
		}
	}
}

func (s *journalSubscriber[E]) next() (E, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for len(s.queue) == 0 && !s.finishing && !s.aborted {
		s.ready.Wait()
	}
	if s.aborted || len(s.queue) == 0 && s.finishing {
		var zero E
		return zero, false
	}
	ev := s.queue[0]
	var zero E
	s.queue[0] = zero
	s.queue = s.queue[1:]
	if !ev.Durable() && !ev.Terminal() {
		s.queuedLive--
	}
	return ev, true
}
