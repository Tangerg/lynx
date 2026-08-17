package agentexec

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
)

type interactionWaitingSubtreeChange struct {
	mu sync.Mutex

	session    *interactionSession
	prepared   *agent.PreparedWaitingSubtreeCancellation
	checkpoint runs.ExecutorCheckpoint
	canceled   []agent.ProcessID
	paused     []agent.ProcessID
	retired    []*managedDelegateCall
	stopExpiry func() bool
	state      interactionWaitingSubtreeChangeState
}

type interactionWaitingSubtreeChangeState uint8

const (
	interactionWaitingSubtreeChangePrepared interactionWaitingSubtreeChangeState = iota
	interactionWaitingSubtreeChangeDiscarded
	interactionWaitingSubtreeChangeApplyFailed
	interactionWaitingSubtreeChangeWaiting
	interactionWaitingSubtreeChangeContinuationReady
	interactionWaitingSubtreeChangeContinued
)

func (state interactionWaitingSubtreeChangeState) String() string {
	switch state {
	case interactionWaitingSubtreeChangePrepared:
		return "prepared"
	case interactionWaitingSubtreeChangeDiscarded:
		return "discarded"
	case interactionWaitingSubtreeChangeApplyFailed:
		return "apply_failed"
	case interactionWaitingSubtreeChangeWaiting:
		return "waiting"
	case interactionWaitingSubtreeChangeContinuationReady:
		return "continuation_ready"
	case interactionWaitingSubtreeChangeContinued:
		return "continued"
	default:
		return fmt.Sprintf("unknown(%d)", state)
	}
}
func (change *interactionWaitingSubtreeChange) Apply(
	disposition runs.WaitingSubtreeDisposition,
) error {
	if change == nil || change.session == nil || change.prepared == nil {
		return errors.New("agentexec: invalid waiting Interaction subtree change")
	}
	switch disposition {
	case runs.WaitingSubtreeStaysWaiting, runs.WaitingSubtreeResumesRunning:
	default:
		return fmt.Errorf("agentexec: invalid waiting subtree disposition %d", disposition)
	}
	change.mu.Lock()
	defer change.mu.Unlock()
	if change.state != interactionWaitingSubtreeChangePrepared {
		return fmt.Errorf(
			"agentexec: waiting Interaction subtree change cannot be applied from state %s",
			change.state,
		)
	}
	change.session.childProjection.lock()
	defer change.session.childProjection.unlock()
	change.stopExpirationLocked()
	if err := change.session.beginSubtreeApplication(change); err != nil {
		return err
	}
	// The Application transaction is already authoritative. Agent Framework staged every
	// fallible Process change during Prepare, so its apply gate cannot be revoked
	// by the request that initiated the product command.
	err := change.prepared.Apply()
	if err == nil {
		change.session.commitSubtreeApplication(change)
	}
	if err != nil {
		if discardErr := change.prepared.Discard(); discardErr != nil &&
			!errors.Is(discardErr, agent.ErrPreparedWaitingSubtreeCancellationResolved) {
			err = errors.Join(err, fmt.Errorf("discard failed prepared Interaction subtree: %w", discardErr))
		}
	}
	switch {
	case err != nil:
		change.state = interactionWaitingSubtreeChangeApplyFailed
	case disposition == runs.WaitingSubtreeStaysWaiting:
		change.state = interactionWaitingSubtreeChangeWaiting
	default:
		change.state = interactionWaitingSubtreeChangeContinuationReady
	}
	change.session.finishSubtreeApplication(change, disposition, err)
	if err != nil {
		return fmt.Errorf("agentexec: apply waiting Interaction subtree: %w", err)
	}
	return nil
}

func (change *interactionWaitingSubtreeChange) Continue(ctx context.Context) error {
	if change == nil || change.session == nil || change.prepared == nil {
		return errors.New("agentexec: invalid waiting Interaction subtree change")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	change.mu.Lock()
	defer change.mu.Unlock()
	if change.state != interactionWaitingSubtreeChangeContinuationReady {
		return fmt.Errorf(
			"agentexec: waiting Interaction subtree change cannot continue from state %s",
			change.state,
		)
	}
	change.state = interactionWaitingSubtreeChangeContinued
	// Continue is the first operation that can advance the Process tree after the
	// Application durably opened the replacement product Segment.
	change.session.segmentClock.start()
	resumeCtx, cancelResume := context.WithTimeout(ctx, authoritativeProjectionTimeout)
	err := change.session.resumePausedProcesses(resumeCtx, change.paused)
	cancelResume()
	change.session.finishSubtreeContinuation(change, err)
	if err != nil {
		return fmt.Errorf("agentexec: continue applied waiting Interaction subtree: %w", err)
	}
	return nil
}

func (change *interactionWaitingSubtreeChange) Discard() error {
	if change == nil || change.session == nil || change.prepared == nil {
		return errors.New("agentexec: invalid waiting Interaction subtree change")
	}
	change.mu.Lock()
	defer change.mu.Unlock()
	if change.state != interactionWaitingSubtreeChangePrepared {
		return nil
	}
	change.stopExpirationLocked()
	if err := change.prepared.Discard(); err != nil {
		return fmt.Errorf("agentexec: discard waiting Interaction subtree: %w", err)
	}
	change.state = interactionWaitingSubtreeChangeDiscarded
	change.session.finishSubtreeDiscard(change)
	return nil
}

func (change *interactionWaitingSubtreeChange) armExpiration(ctx context.Context) {
	change.mu.Lock()
	defer change.mu.Unlock()
	if change.state != interactionWaitingSubtreeChangePrepared {
		return
	}
	change.stopExpiry = context.AfterFunc(ctx, func() {
		_ = change.Discard()
	})
}

// stopExpirationLocked disarms the preparation lease while change.mu is held.
func (change *interactionWaitingSubtreeChange) stopExpirationLocked() {
	if change.stopExpiry != nil {
		change.stopExpiry()
		change.stopExpiry = nil
	}
}

func (session *interactionSession) beginSubtreeApplication(
	change *interactionWaitingSubtreeChange,
) error {
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.finished || session.state.boundary != interactionBoundarySubtreePrepared ||
		session.state.subtreeChange != change {
		return runs.ErrExecutionClaimed
	}
	managedCalls := make([]*managedDelegateCall, len(change.canceled))
	for index, processID := range change.canceled {
		managed := session.state.delegateChildren[processID]
		if managed == nil {
			return fmt.Errorf("agentexec: canceled Interaction member %s has no Delegate binding", processID)
		}
		if session.state.delegateChildren[managed.childProcessID] != managed ||
			session.state.delegateCalls[managed.identity] != managed {
			return errors.New("agentexec: canceled Delegate binding changed before subtree application")
		}
		managedCalls[index] = managed
	}
	session.state.boundary = interactionBoundarySubtreeApplying
	change.retired = managedCalls
	return nil
}

func (session *interactionSession) commitSubtreeApplication(
	change *interactionWaitingSubtreeChange,
) {
	for _, managed := range change.retired {
		managed.mu.Lock()
		managed.parentToolFinished = true
		managed.assistantProjected = true
		managed.segmentProjected = true
		managed.mu.Unlock()
	}
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	session.state.waitingCheckpoint = change.checkpoint.Clone()
	for _, managed := range change.retired {
		delete(session.state.delegateChildren, managed.childProcessID)
		delete(session.state.delegateCalls, managed.identity)
		session.committedReplies.forget(managed.childProcessID)
	}
}

func (session *interactionSession) finishSubtreeApplication(
	change *interactionWaitingSubtreeChange,
	disposition runs.WaitingSubtreeDisposition,
	applyErr error,
) {
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.subtreeChange != change || session.state.boundary != interactionBoundarySubtreeApplying {
		return
	}
	if applyErr != nil {
		session.state.subtreeChange = nil
		session.state.boundary = interactionBoundarySubtreeRecovery
		return
	}
	if disposition == runs.WaitingSubtreeStaysWaiting {
		session.state.subtreeChange = nil
		session.state.boundary = interactionBoundaryWaiting
		return
	}
	session.state.boundary = interactionBoundarySubtreeApplied
}

func (session *interactionSession) finishSubtreeContinuation(
	change *interactionWaitingSubtreeChange,
	continuationErr error,
) {
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.subtreeChange != change || session.state.boundary != interactionBoundarySubtreeApplied {
		return
	}
	session.state.subtreeChange = nil
	if continuationErr != nil {
		session.state.boundary = interactionBoundarySubtreeRecovery
		return
	}
	session.state.boundary = interactionBoundaryInactive
	session.state.waitingCheckpoint = runs.ExecutorCheckpoint{}
}

func (session *interactionSession) finishSubtreeDiscard(change *interactionWaitingSubtreeChange) {
	session.state.mu.Lock()
	defer session.state.mu.Unlock()
	if session.state.subtreeChange != change || session.state.boundary != interactionBoundarySubtreePrepared {
		return
	}
	session.state.subtreeChange = nil
	session.state.boundary = interactionBoundaryWaiting
}

func (session *interactionSession) discardPreparedSubtree(ctx context.Context) error {
	for {
		session.state.mu.Lock()
		boundary := session.state.boundary
		preparedSignal := session.state.subtreePrepared
		change := session.state.subtreeChange
		session.state.mu.Unlock()
		if boundary == interactionBoundarySubtreePreparing && preparedSignal != nil {
			select {
			case <-preparedSignal:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if change == nil {
			return nil
		}
		return change.Discard()
	}
}
