package runs

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// stagedExecutionHandoff is the unique owner of an executor after staging and
// before a Segment accepts it. Transfer is irreversible; abandonment releases
// the exact staged identity on a request-detached cleanup context.
type stagedExecutionHandoff struct {
	lifecycle *segmentLifecycle
	ref       ExecutorRef
	owned     bool
}

func (lifecycle *segmentLifecycle) ownStagedExecution(ref ExecutorRef) *stagedExecutionHandoff {
	return &stagedExecutionHandoff{lifecycle: lifecycle, ref: ref, owned: true}
}

func (handoff *stagedExecutionHandoff) validateFor(sessionID string) error {
	return handoff.ref.ValidateFor(sessionID)
}

func (handoff *stagedExecutionHandoff) transfer() ExecutorRef {
	if !handoff.owned {
		panic("runs: staged execution ownership transferred more than once")
	}
	handoff.owned = false
	return handoff.ref
}

func (handoff *stagedExecutionHandoff) abandon(
	ctx context.Context,
	cause error,
	description string,
) error {
	if handoff == nil || !handoff.owned {
		return cause
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	return handoff.abandonWithin(cleanupCtx, cause, description)
}

func (handoff *stagedExecutionHandoff) abandonWithin(
	ctx context.Context,
	cause error,
	description string,
) error {
	if handoff == nil || !handoff.owned {
		return cause
	}
	handoff.owned = false
	if err := handoff.lifecycle.release(ctx, handoff.ref); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf("runs: release %s %q: %w", description, handoff.ref.ExecutorID, err),
		)
	}
	return cause
}

// claimedResumeAttempt owns the durable claim and, once staged, its executor
// hand-off. A failed claim is made durably lost before its executor is released;
// an accepted Segment consumes the staged executor instead.
type claimedResumeAttempt struct {
	pending      Pending
	terminations RunTerminationCommitter
	nowUTC       func() time.Time
	staged       *stagedExecutionHandoff
	settled      bool
}

func (c *Coordinator) ownClaimedResume(pending Pending) *claimedResumeAttempt {
	return &claimedResumeAttempt{
		pending:      pending,
		terminations: c.terminations,
		nowUTC:       c.publications.nowUTC,
	}
}

func (attempt *claimedResumeAttempt) ownStagedExecution(
	lifecycle *segmentLifecycle,
	ref ExecutorRef,
) {
	if attempt.staged != nil {
		panic("runs: claimed resume staged more than one execution")
	}
	attempt.staged = lifecycle.ownStagedExecution(ref)
}

func (attempt *claimedResumeAttempt) accept() {
	if attempt.settled {
		panic("runs: claimed resume settled more than once")
	}
	if attempt.staged == nil {
		panic("runs: claimed resume accepted without a staged execution")
	}
	attempt.staged.transfer()
	attempt.settled = true
}

func (attempt *claimedResumeAttempt) fail(ctx context.Context, cause error) error {
	if attempt.settled {
		return cause
	}
	if cause == nil {
		cause = errors.New("runs: claimed resume exited without accepting its execution")
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if err := attempt.terminations.ApplyRunLost(
		cleanupCtx,
		attempt.pending.SessionID,
		attempt.pending.RootRunID,
		attempt.nowUTC(),
	); err != nil {
		// The durable waiting boundary still names this executor. Releasing it
		// before RunLost commits would leave that authoritative state unusable.
		return errors.Join(
			cause,
			fmt.Errorf(
				"runs: recover claimed resume %q as lost: %w",
				attempt.pending.RootRunID,
				err,
			),
		)
	}
	attempt.settled = true
	result := fmt.Errorf("%w: %w", ErrRunNotFound, cause)
	return attempt.staged.abandonWithin(cleanupCtx, result, "lost continuation")
}
