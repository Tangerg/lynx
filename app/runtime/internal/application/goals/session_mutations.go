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
func (s *SessionMutations) acquire(sessionIDs ...string) func() {
	releaseSessions := s.acquireSessions(sessionIDs...)
	s.admission.RLock()
	return func() {
		s.admission.RUnlock()
		releaseSessions()
	}
}

// acquireSessions is the internal half of acquire. Background reconciliation
// participates in per-session ordering but not external command admission; its
// task-group ownership is the shutdown join boundary.
func (s *SessionMutations) acquireSessions(sessionIDs ...string) func() {
	ids := normalizeSessionIDs(sessionIDs)
	s.mu.Lock()
	if s.commandLocks == nil {
		s.commandLocks = make(map[string]*sessionCommandLock)
	}
	locks := make([]*sessionCommandLock, 0, len(ids))
	for _, sessionID := range ids {
		lock := s.commandLocks[sessionID]
		if lock == nil {
			lock = &sessionCommandLock{}
			s.commandLocks[sessionID] = lock
		}
		lock.refs++
		locks = append(locks, lock)
	}
	s.mu.Unlock()

	for _, lock := range locks {
		lock.mu.Lock()
	}
	return func() {
		for _, lock := range slices.Backward(locks) {
			lock.mu.Unlock()
		}
		s.mu.Lock()
		for i, sessionID := range ids {
			lock := locks[i]
			lock.refs--
			if lock.refs == 0 {
				delete(s.commandLocks, sessionID)
			}
		}
		s.mu.Unlock()
	}
}

func normalizeSessionIDs(sessionIDs []string) []string {
	ids := slices.Clone(sessionIDs)
	slices.Sort(ids)
	return slices.Compact(ids)
}

func (s *SessionMutations) acquireAll() func() {
	s.admission.Lock()
	return s.admission.Unlock
}

// WithSessionMutation owns both phases of a session mutation. A failed commit
// leaves the authoritative Goal drive intact. Once commit succeeds, affected
// drives are quiesced and afterCommit is always attempted; failures from either
// post-commit phase are reported together.
func (s *SessionMutations) WithSessionMutation(
	ctx context.Context,
	sessionIDs []string,
	commit func(context.Context) error,
	afterCommit func(context.Context) error,
) error {
	sessionIDs = normalizeSessionIDs(sessionIDs)
	release := s.acquire(sessionIDs...)
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
		if drive := s.quiesce(sessionID); drive != nil {
			drives = append(drives, ownedDrive{sessionID: sessionID, drive: drive})
		}
	}
	var errs []error
	for _, owned := range drives {
		errs = append(errs, owned.drive.await(ctx))
		if owned.drive.completed() {
			s.forget(owned.sessionID, owned.drive)
		}
	}
	errs = append(errs, afterCommit(ctx))
	return errors.Join(errs...)
}

func (s *SessionMutations) launch(sessionID string, drive *goalDrive) {
	s.mu.Lock()
	if s.drives == nil {
		s.drives = map[string]*goalDrive{}
	}
	if s.drives[sessionID] != nil {
		s.mu.Unlock()
		panic("goals: launch attempted before the prior Goal drive was joined")
	}
	s.drives[sessionID] = drive
	s.mu.Unlock()
}

func (s *SessionMutations) forget(sessionID string, drive *goalDrive) {
	s.mu.Lock()
	if s.drives[sessionID] == drive {
		delete(s.drives, sessionID)
	}
	s.mu.Unlock()
}

func (s *SessionMutations) quiesce(sessionID string) *goalDrive {
	s.mu.Lock()
	drive := s.drives[sessionID]
	s.mu.Unlock()
	if drive != nil {
		drive.quiesce()
	}
	return drive
}

func (s *SessionMutations) activeDrive(sessionID string) *goalDrive {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.drives[sessionID]
}
