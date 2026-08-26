package sessions

import (
	"context"
	"sync"
)

// Admissions is the session lifecycle's view of the shared run and
// working-tree admission state. A file rollback's `git reset --hard` must see
// both a sibling's segment admission and its already-live run on the same cwd.
type Admissions interface {
	AcquireSession(sessionID string) (release func(), ok bool)
	AcquireWorkingTreeMutation(cwd string) (release func(), ok bool)
}

// WorkingTreeAdmission is a held working-tree slot. Release is idempotent
// across value copies.
type WorkingTreeAdmission struct {
	release *releaseOnce
}

// Release drops the held working-tree slot.
func (w WorkingTreeAdmission) Release() {
	if w.release != nil {
		w.release.run()
	}
}

// Admission is a held single-writer slot. Release is idempotent across
// value copies.
type Admission struct {
	SessionID string
	release   *releaseOnce
}

// Release drops the held single-writer slot.
func (a Admission) Release() {
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

// heldSessionAdmission builds a slot whose Release drops its owned
// single-writer claim exactly once.
func heldSessionAdmission(sessionID string, release func()) Admission {
	return Admission{
		SessionID: sessionID,
		release:   newReleaseOnce(release),
	}
}

func heldWorkingTreeAdmission(release func()) WorkingTreeAdmission {
	return WorkingTreeAdmission{release: newReleaseOnce(release)}
}

// ClaimIdleSession reserves a Session only when it has neither an in-process
// writer nor a parked Run. Use it for operations whose result cannot remain
// coherent with an existing executor continuation, such as export, import, or
// editing execution workspace policy.
func (c *Coordinator) ClaimIdleSession(ctx context.Context, sessionID string) (Admission, error) {
	release, ok := c.admissions.AcquireSession(sessionID)
	if !ok {
		return Admission{}, ErrSessionBusy
	}
	admission := heldSessionAdmission(sessionID, release)
	open, err := c.interrupts.List(ctx, sessionID)
	if err != nil {
		admission.Release()
		return Admission{}, err
	}
	if len(open) > 0 {
		admission.Release()
		return Admission{}, ErrSessionBusy
	}
	return admission, nil
}

// ClaimSessionMutation reserves a Session for a lifecycle write-set that
// explicitly consumes or terminalizes any parked Run it finds. It deliberately
// does not reject open interrupts; callers must own that disposition atomically.
func (c *Coordinator) ClaimSessionMutation(sessionID string) (Admission, error) {
	release, ok := c.admissions.AcquireSession(sessionID)
	if !ok {
		return Admission{}, ErrSessionBusy
	}
	return heldSessionAdmission(sessionID, release), nil
}
