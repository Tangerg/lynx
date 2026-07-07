package lifecycle

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupts"
)

// SessionClaimer is the run-admission slot used to enforce one writer per
// session across active runs and start/resume races.
type SessionClaimer interface {
	ClaimSession(sessionID string) bool
	ReleaseSession(sessionID string)
}

// RunAdmission is a held single-writer slot. Release must be called exactly
// once by the caller after the run segment is registered or admission fails.
type RunAdmission struct {
	SessionID string
	release   func()
}

// Release drops the held single-writer slot.
func (a RunAdmission) Release() {
	if a.release != nil {
		a.release()
	}
}

// ClaimRunSlot reserves a session's single-writer slot for a fresh run and
// rejects sessions already parked on an open interrupt.
func (c *Coordinator) ClaimRunSlot(ctx context.Context, claims SessionClaimer, sessionID string) (RunAdmission, error) {
	if !claims.ClaimSession(sessionID) {
		return RunAdmission{}, ErrSessionBusy
	}
	admission := RunAdmission{
		SessionID: sessionID,
		release:   func() { claims.ReleaseSession(sessionID) },
	}
	open, err := c.s.Interrupts().List(ctx, sessionID)
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
func (c *Coordinator) ClaimMutationSlot(claims SessionClaimer, sessionID string) (RunAdmission, error) {
	if !claims.ClaimSession(sessionID) {
		return RunAdmission{}, ErrSessionBusy
	}
	return RunAdmission{
		SessionID: sessionID,
		release:   func() { claims.ReleaseSession(sessionID) },
	}, nil
}

// ClaimResumeSlot peeks an open interrupt to find its session, then reserves
// that session's single-writer slot before the interrupt is consumed.
func (c *Coordinator) ClaimResumeSlot(ctx context.Context, claims SessionClaimer, parentRunID string) (interrupts.Pending, RunAdmission, error) {
	pending, found, err := c.s.Interrupts().Get(ctx, parentRunID)
	if err != nil {
		return interrupts.Pending{}, RunAdmission{}, err
	}
	if !found {
		return interrupts.Pending{}, RunAdmission{}, ErrInterruptNotOpen
	}
	if !claims.ClaimSession(pending.SessionID) {
		return pending, RunAdmission{}, ErrSessionBusy
	}
	return pending, RunAdmission{
		SessionID: pending.SessionID,
		release:   func() { claims.ReleaseSession(pending.SessionID) },
	}, nil
}
