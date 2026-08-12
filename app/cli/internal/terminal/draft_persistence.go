package terminal

import (
	"errors"
	"sync"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
)

var errDraftPersistenceClosed = errors.New("draft persistence is closed")

type draftRepository interface {
	SaveDraft(string, agent.Message) error
}

type draftSnapshot struct {
	revision  uint64
	sessionID string
	message   agent.Message
	due       time.Time
}

type draftPersistenceResult struct {
	revision uint64
	err      error
}

type draftFlush struct {
	snapshot draftSnapshot
	done     chan error
}

// draftPersistence is the single writer for composer recovery state. Schedule
// only replaces pending work; Flush is a serialization barrier used by session
// transitions before they mutate runtime state.
type draftPersistence struct {
	repository draftRepository
	delay      time.Duration
	notify     func(draftPersistenceResult)

	commandMu sync.Mutex
	mu        sync.Mutex
	pending   *draftSnapshot
	revision  uint64
	closed    bool
	wake      chan struct{}
	flush     chan draftFlush
	shutdown  chan chan error
	done      chan struct{}
}

func newDraftPersistence(
	repository draftRepository,
	delay time.Duration,
	notify func(draftPersistenceResult),
) *draftPersistence {
	if repository == nil {
		return nil
	}
	persistence := &draftPersistence{
		repository: repository,
		delay:      max(delay, 0),
		notify:     notify,
		wake:       make(chan struct{}, 1),
		flush:      make(chan draftFlush),
		shutdown:   make(chan chan error),
		done:       make(chan struct{}),
	}
	go persistence.run()
	return persistence
}

// Schedule records the latest complete authoring value without blocking the
// caller on filesystem latency. A snapshot is cloned at this boundary because
// attachment slices remain owned by the UI model.
func (p *draftPersistence) Schedule(sessionID string, message agent.Message) bool {
	if p == nil || sessionID == "" {
		return false
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return false
	}
	p.revision++
	snapshot := draftSnapshot{
		revision: p.revision, sessionID: sessionID, message: message.Clone(), due: time.Now().Add(p.delay),
	}
	p.pending = &snapshot
	p.mu.Unlock()
	select {
	case p.wake <- struct{}{}:
	default:
	}
	return true
}

// Flush supersedes pending autosave work and waits until every older write has
// finished before saving snapshot. This ordering prevents an older writer from
// winning a rename race after a session transition has committed newer state.
func (p *draftPersistence) Flush(sessionID string, message agent.Message) error {
	if p == nil || sessionID == "" {
		return nil
	}
	p.commandMu.Lock()
	defer p.commandMu.Unlock()
	snapshot, ok := p.reserve(sessionID, message)
	if !ok {
		return errDraftPersistenceClosed
	}
	request := draftFlush{snapshot: snapshot, done: make(chan error, 1)}
	select {
	case p.flush <- request:
		return <-request.done
	case <-p.done:
		return errDraftPersistenceClosed
	}
}

// Current reports whether result still describes the newest requested value.
// UI callbacks use it to prevent a late autosave error from replacing the
// outcome of a newer synchronous flush.
func (p *draftPersistence) Current(revision uint64) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && revision == p.revision
}

// Close prevents new work, flushes the latest pending snapshot without its
// debounce delay, and joins the writer. Callers should Flush the live composer
// first; the pending fallback protects shutdown if that projection is invalid.
func (p *draftPersistence) Close() error {
	if p == nil {
		return nil
	}
	p.commandMu.Lock()
	defer p.commandMu.Unlock()
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		<-p.done
		return nil
	}
	p.closed = true
	p.mu.Unlock()

	result := make(chan error, 1)
	select {
	case p.shutdown <- result:
		err := <-result
		<-p.done
		return err
	case <-p.done:
		return nil
	}
}

func (p *draftPersistence) reserve(sessionID string, message agent.Message) (draftSnapshot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return draftSnapshot{}, false
	}
	p.revision++
	snapshot := draftSnapshot{revision: p.revision, sessionID: sessionID, message: message.Clone()}
	return snapshot, true
}

func (p *draftPersistence) discardPendingThrough(revision uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending != nil && p.pending.revision <= revision {
		p.pending = nil
	}
}

func (p *draftPersistence) takePending() (draftSnapshot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		return draftSnapshot{}, false
	}
	snapshot := *p.pending
	p.pending = nil
	return snapshot, true
}

func (p *draftPersistence) pendingDelay(now time.Time) (time.Duration, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		return 0, false
	}
	return max(p.pending.due.Sub(now), 0), true
}

func (p *draftPersistence) takeDue(now time.Time) (draftSnapshot, time.Duration, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		return draftSnapshot{}, 0, false
	}
	if remaining := p.pending.due.Sub(now); remaining > 0 {
		return draftSnapshot{}, remaining, false
	}
	snapshot := *p.pending
	p.pending = nil
	return snapshot, 0, true
}

func (p *draftPersistence) run() {
	defer close(p.done)
	var timer *time.Timer
	var elapsed <-chan time.Time
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		elapsed = nil
	}
	resetTimer := func(delay time.Duration) {
		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			stopTimer()
			timer.Reset(delay)
		}
		elapsed = timer.C
	}
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	for {
		select {
		case <-p.wake:
			if delay, pending := p.pendingDelay(time.Now()); pending {
				resetTimer(delay)
			} else {
				stopTimer()
			}
		case <-elapsed:
			elapsed = nil
			snapshot, remaining, due := p.takeDue(time.Now())
			if due {
				p.publish(snapshot, p.save(snapshot))
			} else if remaining > 0 {
				resetTimer(remaining)
			}
		case request := <-p.flush:
			stopTimer()
			p.discardPendingThrough(request.snapshot.revision)
			request.done <- p.save(request.snapshot)
			if delay, pending := p.pendingDelay(time.Now()); pending {
				resetTimer(delay)
			}
		case result := <-p.shutdown:
			stopTimer()
			var err error
			if snapshot, ok := p.takePending(); ok {
				err = p.save(snapshot)
			}
			result <- err
			return
		}
	}
}

func (p *draftPersistence) save(snapshot draftSnapshot) error {
	return p.repository.SaveDraft(snapshot.sessionID, snapshot.message)
}

func (p *draftPersistence) publish(snapshot draftSnapshot, err error) {
	if p.notify != nil {
		p.notify(draftPersistenceResult{revision: snapshot.revision, err: err})
	}
}
