package goals

import (
	"context"
	"errors"
	"slices"
	"sync"
)

// SessionMutations serializes session lifecycle write-sets with Goal commands
// and owns the in-process registry of active Goal drives. It is created before
// either coordinator so both use cases share one stable lifecycle boundary
// without late-bound references.
type SessionMutations struct {
	admission sync.RWMutex

	mu           sync.Mutex
	commandLocks map[string]*sessionCommandLock
	drives       map[string]*goalDrive
}

type sessionCommandLock struct {
	mu   sync.Mutex
	refs int
}

// NewSessionMutations returns the shared lifecycle coordinator for one runtime.
func NewSessionMutations() *SessionMutations {
	return &SessionMutations{drives: map[string]*goalDrive{}}
}

// acquire serializes lifecycle commands only for the sessions they mutate.
// Session locks are taken before admission so a command waiting behind internal
// Goal drive reconciliation does not prevent shutdown from closing task admission.
// Once the read side is held, shutdown cannot cross the command's launch
// boundary.
func (m *SessionMutations) acquire(sessionIDs ...string) func() {
	releaseSessions := m.acquireSessions(sessionIDs...)
	m.admission.RLock()
	return func() {
		m.admission.RUnlock()
		releaseSessions()
	}
}

// acquireSessions is the internal half of acquire. Background reconciliation
// participates in per-session ordering but not external command admission; its
// task-group ownership is the shutdown join boundary.
func (m *SessionMutations) acquireSessions(sessionIDs ...string) func() {
	ids := normalizeSessionIDs(sessionIDs)
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
		for _, lock := range slices.Backward(locks) {
			lock.mu.Unlock()
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
// leaves the authoritative Goal drive intact. Once commit succeeds, affected
// drives are quiesced and afterCommit is always attempted; failures from either
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
	type ownedDrive struct {
		sessionID string
		drive     *goalDrive
	}
	drives := make([]ownedDrive, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if drive := m.quiesce(sessionID); drive != nil {
			drives = append(drives, ownedDrive{sessionID: sessionID, drive: drive})
		}
	}
	var errs []error
	for _, owned := range drives {
		errs = append(errs, owned.drive.await(ctx))
		if owned.drive.completed() {
			m.forget(owned.sessionID, owned.drive)
		}
	}
	errs = append(errs, afterCommit(ctx))
	return errors.Join(errs...)
}

func (m *SessionMutations) launch(sessionID string, drive *goalDrive) {
	m.mu.Lock()
	if m.drives == nil {
		m.drives = map[string]*goalDrive{}
	}
	if m.drives[sessionID] != nil {
		m.mu.Unlock()
		panic("goals: launch attempted before the prior Goal drive was joined")
	}
	m.drives[sessionID] = drive
	m.mu.Unlock()
}

func (m *SessionMutations) forget(sessionID string, drive *goalDrive) {
	m.mu.Lock()
	if m.drives[sessionID] == drive {
		delete(m.drives, sessionID)
	}
	m.mu.Unlock()
}

func (m *SessionMutations) quiesce(sessionID string) *goalDrive {
	m.mu.Lock()
	drive := m.drives[sessionID]
	m.mu.Unlock()
	if drive != nil {
		drive.quiesce()
	}
	return drive
}

func (m *SessionMutations) activeDrive(sessionID string) *goalDrive {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.drives[sessionID]
}
