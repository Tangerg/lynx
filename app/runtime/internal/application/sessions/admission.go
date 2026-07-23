package sessions

import (
	"context"
	"errors"
	"sync"
)

// SessionAdmissions is the run-admission slot used to enforce one writer per
// session across active runs and start/resume races. ActiveSessionWithCwd
// widens the guard to a shared working tree: a file rollback's `git reset
// --hard` writes the tree a sibling session sharing the cwd would race, so the
// mutation must see any in-flight run on that tree, not just this session.
type SessionAdmissions interface {
	AcquireSession(sessionID string) (release func(), ok bool)
	ActiveSessionWithCwd(cwd string) string
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
