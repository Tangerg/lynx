package terminal

import (
	"errors"
	"sync"

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
}

type draftPersistenceResult struct {
	revision uint64
	err      error
}

type draftPersistence struct {
	repository draftRepository
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

type draftFlush struct {
	snapshot draftSnapshot
	done     chan error
}

// draftPersistence is the single filesystem writer for composer recovery
// state. Schedule only replaces pending work; Flush is a serialization barrier
// used by session transitions before they mutate runtime state.
func newDraftPersistence(repository draftRepository, notify func(draftPersistenceResult)) *draftPersistence {
	if repository == nil {
		return nil
	}
	persistence := &draftPersistence{
		repository: repository,
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
	snapshot := draftSnapshot{revision: p.revision, sessionID: sessionID, message: message.Clone()}
	p.pending = &snapshot
	p.mu.Unlock()
	p.signal()
	return true
}

// Flush supersedes older pending work and waits until every older write has
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

func (p *draftPersistence) Current(revision uint64) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return !p.closed && revision == p.revision
}

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
	return draftSnapshot{revision: p.revision, sessionID: sessionID, message: message.Clone()}, true
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

func (p *draftPersistence) discardPendingThrough(revision uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending != nil && p.pending.revision <= revision {
		p.pending = nil
	}
}

func (p *draftPersistence) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *draftPersistence) run() {
	defer close(p.done)
	for {
		select {
		case <-p.wake:
			if snapshot, ok := p.takePending(); ok {
				p.publish(snapshot, p.save(snapshot))
			}
		case request := <-p.flush:
			p.discardPendingThrough(request.snapshot.revision)
			request.done <- p.save(request.snapshot)
			if snapshot, ok := p.takePending(); ok {
				p.publish(snapshot, p.save(snapshot))
			}
		case result := <-p.shutdown:
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
