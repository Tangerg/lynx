package sessions

import (
	"context"
	"errors"
	"sync"
)

// SessionAdmissions is the session lifecycle's view of the shared run and
// working-tree admission state. A file rollback's `git reset --hard` must see
// both a sibling's segment admission and its already-live run on the same cwd.
type SessionAdmissions interface {
	AcquireSession(sessionID string) (release func(), ok bool)
	AcquireWorkingTreeRun(cwd string) (release func(), ok bool)
	AcquireWorkingTreeMutation(cwd string) (release func(), ok bool)
	ActiveSessions() map[string]bool
}

// WorkingTreeAdmission is a held working-tree slot. Release is idempotent
// across value copies.
type WorkingTreeAdmission struct {
	release *releaseOnce
}

// Release drops the held working-tree slot.
func (a WorkingTreeAdmission) Release() {
	if a.release != nil {
		a.release.run()
	}
}

// RunAdmission is a held single-writer slot. Release is idempotent across value
// copies and should be called after the run segment is registered or admission
// fails.
type RunAdmission struct {
	SessionID string
	release   *releaseOnce
}

// Release drops the held single-writer slot.
func (a RunAdmission) Release() {
	if a.release != nil {
		a.release.run()
	}
}

type releaseOnce struct {
	once sync.Once
	fn   func()
}

func newReleaseOnce(fn func()) *releaseOnce { return &releaseOnce{fn: fn} }

func (r *releaseOnce) run() {
	r.once.Do(r.fn)
}

// heldAdmission builds a slot whose Release drops its owned single-writer claim
// exactly once.
func heldAdmission(sessionID string, release func()) RunAdmission {
	return RunAdmission{
		SessionID: sessionID,
		release:   newReleaseOnce(release),
	}
}

func heldWorkingTreeAdmission(release func()) WorkingTreeAdmission {
	return WorkingTreeAdmission{release: newReleaseOnce(release)}
}

// ClaimRunSlot reserves a session's single-writer slot for a fresh run and
// rejects sessions already parked on an open interrupt.
func (c *Coordinator) ClaimRunSlot(ctx context.Context, sessionID string) (RunAdmission, error) {
	release, ok := c.admissions.AcquireSession(sessionID)
	if !ok {
		return RunAdmission{}, ErrSessionBusy
	}
	admission := heldAdmission(sessionID, release)
	if c.interrupts == nil {
		admission.Release()
		return RunAdmission{}, errors.New("sessions: interrupt store is unavailable")
	}
	open, err := c.interrupts.List(ctx, sessionID)
	if err != nil {
		admission.Release()
		return RunAdmission{}, err
	}
	if len(open) > 0 {
		admission.Release()
		return RunAdmission{}, ErrSessionBusy
	}
	return admission, nil
}

// ClaimMutationSlot reserves a session's single-writer slot for a destructive
// session mutation. Unlike [Coordinator.ClaimRunSlot], it does not reject open
// interrupts: rollback/delete/import decide what to do with parked runs inside
// their own lifecycle write-set.
func (c *Coordinator) ClaimMutationSlot(sessionID string) (RunAdmission, error) {
	release, ok := c.admissions.AcquireSession(sessionID)
	if !ok {
		return RunAdmission{}, ErrSessionBusy
	}
	return heldAdmission(sessionID, release), nil
}
