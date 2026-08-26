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

func (s *segmentLifecycle) ownStagedExecution(ref ExecutorRef) *stagedExecutionHandoff {
	return &stagedExecutionHandoff{lifecycle: s, ref: ref, owned: true}
}

func (s *stagedExecutionHandoff) validateFor(sessionID string) error {
	return s.ref.ValidateFor(sessionID)
}

func (s *stagedExecutionHandoff) transfer() ExecutorRef {
	if !s.owned {
		panic("runs: staged execution ownership transferred more than once")
	}
	s.owned = false
	return s.ref
}

func (s *stagedExecutionHandoff) abandon(
	ctx context.Context,
	cause error,
	description string,
) error {
	if s == nil || !s.owned {
		return cause
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	return s.abandonWithin(cleanupCtx, cause, description)
}

func (s *stagedExecutionHandoff) abandonWithin(
	ctx context.Context,
	cause error,
	description string,
) error {
	if s == nil || !s.owned {
		return cause
	}
	s.owned = false
	if err := s.lifecycle.release(ctx, s.ref); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf("runs: release %s %q: %w", description, s.ref.ExecutorID, err),
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

func (c *claimedResumeAttempt) ownStagedExecution(
	lifecycle *segmentLifecycle,
	ref ExecutorRef,
) {
	if c.staged != nil {
		panic("runs: claimed resume staged more than one execution")
	}
	c.staged = lifecycle.ownStagedExecution(ref)
}

func (c *claimedResumeAttempt) accept() {
	if c.settled {
		panic("runs: claimed resume settled more than once")
	}
	if c.staged == nil {
		panic("runs: claimed resume accepted without a staged execution")
	}
	c.staged.transfer()
	c.settled = true
}

func (c *claimedResumeAttempt) fail(ctx context.Context, cause error) error {
	if c.settled {
		return cause
	}
	if cause == nil {
		cause = errors.New("runs: claimed resume exited without accepting its execution")
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), runCleanupTimeout)
	defer cancel()
	if err := c.terminations.ApplyClaimedRunLost(
		cleanupCtx,
		c.pending,
		c.nowUTC(),
	); err != nil {
		// The durable waiting boundary still names this executor. Releasing it
		// before RunLost commits would leave that authoritative state unusable.
		return errors.Join(
			cause,
			fmt.Errorf(
				"runs: recover claimed resume %q as lost: %w",
				c.pending.RootRunID,
				err,
			),
		)
	}
	c.settled = true
	result := fmt.Errorf("%w: %w", ErrRunNotFound, cause)
	return c.staged.abandonWithin(cleanupCtx, result, "lost continuation")
}
