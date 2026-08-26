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
func (d *draftPersistence) Schedule(sessionID string, message agent.Message) bool {
	if d == nil || sessionID == "" {
		return false
	}
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		return false
	}
	d.revision++
	snapshot := draftSnapshot{revision: d.revision, sessionID: sessionID, message: message.Clone()}
	d.pending = &snapshot
	d.mu.Unlock()
	d.signal()
	return true
}

// Flush supersedes older pending work and waits until every older write has
// finished before saving snapshot. This ordering prevents an older writer from
// winning a rename race after a session transition has committed newer state.
func (d *draftPersistence) Flush(sessionID string, message agent.Message) error {
	if d == nil || sessionID == "" {
		return nil
	}
	d.commandMu.Lock()
	defer d.commandMu.Unlock()
	snapshot, ok := d.reserve(sessionID, message)
	if !ok {
		return errDraftPersistenceClosed
	}
	request := draftFlush{snapshot: snapshot, done: make(chan error, 1)}
	select {
	case d.flush <- request:
		return <-request.done
	case <-d.done:
		return errDraftPersistenceClosed
	}
}

func (d *draftPersistence) Current(revision uint64) bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.closed && revision == d.revision
}

func (d *draftPersistence) Close() error {
	if d == nil {
		return nil
	}
	d.commandMu.Lock()
	defer d.commandMu.Unlock()
	d.mu.Lock()
	if d.closed {
		d.mu.Unlock()
		<-d.done
		return nil
	}
	d.closed = true
	d.mu.Unlock()

	result := make(chan error, 1)
	select {
	case d.shutdown <- result:
		err := <-result
		<-d.done
		return err
	case <-d.done:
		return nil
	}
}

func (d *draftPersistence) reserve(sessionID string, message agent.Message) (draftSnapshot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return draftSnapshot{}, false
	}
	d.revision++
	return draftSnapshot{revision: d.revision, sessionID: sessionID, message: message.Clone()}, true
}

func (d *draftPersistence) takePending() (draftSnapshot, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending == nil {
		return draftSnapshot{}, false
	}
	snapshot := *d.pending
	d.pending = nil
	return snapshot, true
}

func (d *draftPersistence) discardPendingThrough(revision uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pending != nil && d.pending.revision <= revision {
		d.pending = nil
	}
}

func (d *draftPersistence) signal() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *draftPersistence) run() {
	defer close(d.done)
	for {
		select {
		case <-d.wake:
			if snapshot, ok := d.takePending(); ok {
				d.publish(snapshot, d.save(snapshot))
			}
		case request := <-d.flush:
			d.discardPendingThrough(request.snapshot.revision)
			request.done <- d.save(request.snapshot)
			if snapshot, ok := d.takePending(); ok {
				d.publish(snapshot, d.save(snapshot))
			}
		case result := <-d.shutdown:
			var err error
			if snapshot, ok := d.takePending(); ok {
				err = d.save(snapshot)
			}
			result <- err
			return
		}
	}
}

func (d *draftPersistence) save(snapshot draftSnapshot) error {
	return d.repository.SaveDraft(snapshot.sessionID, snapshot.message)
}

func (d *draftPersistence) publish(snapshot draftSnapshot, err error) {
	if d.notify != nil {
		d.notify(draftPersistenceResult{revision: snapshot.revision, err: err})
	}
}
