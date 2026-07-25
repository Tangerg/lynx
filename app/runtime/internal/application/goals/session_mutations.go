package goals

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// SessionMutations serializes session lifecycle write-sets with Goal commands
// and owns the in-process registry of active Goal loops. It is created before
// either coordinator, so Session lifecycle coordination never needs a mutable
// Bootstrap proxy to reach a Driver constructed later.
type SessionMutations struct {
	admission sync.RWMutex

	mu           sync.Mutex
	commandLocks map[string]*sessionCommandLock
	running      map[string]*loopHandle
}

type sessionCommandLock struct {
	mu   sync.Mutex
	refs int
}

// NewSessionMutations returns the shared lifecycle coordinator for one runtime.
func NewSessionMutations() *SessionMutations {
	return &SessionMutations{running: map[string]*loopHandle{}}
}

// acquire serializes lifecycle commands only for the sessions they mutate.
// admission's read side lets unrelated sessions progress concurrently; shutdown
// takes its write side to close task admission after every accepted command has
// left its launch boundary.
func (m *SessionMutations) acquire(sessionIDs ...string) func() {
	ids := normalizeSessionIDs(sessionIDs)

	m.admission.RLock()
	m.mu.Lock()
	if m.commandLocks == nil {
		m.commandLocks = make(map[string]*sessionCommandLock)
	}
	locks := make([]*sessionCommandLock, 0, len(ids))
	for _, sessionID := range ids {
		lock := m.commandLocks[sessionID]
		if lock == nil {
			lock = &sessionCommandLock{}
			m.commandLocks[sessionID] = lock
		}
		lock.refs++
		locks = append(locks, lock)
	}
	m.mu.Unlock()

	for _, lock := range locks {
		lock.mu.Lock()
	}
	return func() {
		for i := len(locks) - 1; i >= 0; i-- {
			locks[i].mu.Unlock()
		}
		m.mu.Lock()
		for i, sessionID := range ids {
			lock := locks[i]
			lock.refs--
			if lock.refs == 0 {
				delete(m.commandLocks, sessionID)
			}
		}
		m.mu.Unlock()
		m.admission.RUnlock()
	}
}

func normalizeSessionIDs(sessionIDs []string) []string {
	ids := slices.Clone(sessionIDs)
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (m *SessionMutations) acquireAll() func() {
	m.admission.Lock()
	return m.admission.Unlock
}

// WithSessionMutation owns both phases of a session mutation. A failed commit
// leaves the authoritative Goal loop intact. Once commit succeeds, affected
// loops are quiesced and afterCommit is always attempted; failures from either
// post-commit phase are reported together.
func (m *SessionMutations) WithSessionMutation(
	ctx context.Context,
	sessionIDs []string,
	commit func(context.Context) error,
	afterCommit func(context.Context) error,
) error {
	sessionIDs = normalizeSessionIDs(sessionIDs)
	release := m.acquire(sessionIDs...)
	defer release()
	if err := commit(ctx); err != nil {
		return err
	}
	handles := make([]*loopHandle, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if handle := m.quiesce(sessionID); handle != nil {
			handle.resolveStop(stopLeaveQuiesced)
			handles = append(handles, handle)
		}
	}
	var errs []error
	for _, handle := range handles {
		errs = append(errs, handle.wait(ctx))
	}
	errs = append(errs, afterCommit(ctx))
	return errors.Join(errs...)
}

func (m *SessionMutations) launch(sessionID string, handle *loopHandle) {
	m.mu.Lock()
	if m.running == nil {
		m.running = map[string]*loopHandle{}
	}
	if m.running[sessionID] != nil {
		m.mu.Unlock()
		panic("goals: launch attempted before the prior session driver was joined")
	}
	m.running[sessionID] = handle
	m.mu.Unlock()
}

func (m *SessionMutations) forget(sessionID string, handle *loopHandle) {
	m.mu.Lock()
	if m.running[sessionID] == handle {
		delete(m.running, sessionID)
	}
	m.mu.Unlock()
}

func (m *SessionMutations) quiesce(sessionID string) *loopHandle {
	m.mu.Lock()
	handle := m.running[sessionID]
	if handle != nil && handle.finished() {
		delete(m.running, sessionID)
	}
	m.mu.Unlock()
	if handle != nil {
		handle.quiesce()
	}
	return handle
}

func (m *SessionMutations) driverLease(sessionID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	handle := m.running[sessionID]
	if handle == nil {
		return ""
	}
	return handle.leaseID
}
