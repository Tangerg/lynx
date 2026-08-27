package agentexec

import (
	"context"
	"errors"
	"fmt"
	"sync"

	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
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

func (i interactionWaitingSubtreeChangeState) String() string {
	switch i {
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
		return fmt.Sprintf("unknown(%d)", i)
	}
}
func (i *interactionWaitingSubtreeChange) Apply(
	disposition runs.WaitingSubtreeDisposition,
) error {
	if i == nil || i.session == nil || i.prepared == nil {
		return errors.New("agentexec: invalid waiting Interaction subtree change")
	}
	if !disposition.Valid() {
		return fmt.Errorf("agentexec: invalid waiting subtree disposition %q", disposition)
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangePrepared {
		return fmt.Errorf(
			"agentexec: waiting Interaction subtree change cannot be applied from state %s",
			i.state,
		)
	}
	i.session.childProjection.lock()
	defer i.session.childProjection.unlock()
	i.stopExpirationLocked()
	if err := i.session.beginSubtreeApplication(i); err != nil {
		return err
	}
	// The Application transaction is already authoritative. Agent Framework staged every
	// fallible Process change during Prepare, so its apply gate cannot be revoked
	// by the request that initiated the product command.
	err := i.prepared.Apply()
	if err == nil {
		i.session.commitSubtreeApplication(i)
	}
	if err != nil {
		if discardErr := i.prepared.Discard(); discardErr != nil &&
			!errors.Is(discardErr, agent.ErrPreparedWaitingSubtreeCancellationResolved) {
			err = errors.Join(err, fmt.Errorf("discard failed prepared Interaction subtree: %w", discardErr))
		}
	}
	switch {
	case err != nil:
		i.state = interactionWaitingSubtreeChangeApplyFailed
	case disposition == runs.WaitingSubtreeStaysWaiting:
		i.state = interactionWaitingSubtreeChangeWaiting
	default:
		i.state = interactionWaitingSubtreeChangeContinuationReady
	}
	i.session.finishSubtreeApplication(i, disposition, err)
	if err != nil {
		return fmt.Errorf("agentexec: apply waiting Interaction subtree: %w", err)
	}
	return nil
}

func (i *interactionWaitingSubtreeChange) Continue(ctx context.Context) error {
	if i == nil || i.session == nil || i.prepared == nil {
		return errors.New("agentexec: invalid waiting Interaction subtree change")
	}
	if ctx == nil {
		return errors.New("agentexec: waiting Interaction subtree continuation context is required")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangeContinuationReady {
		return fmt.Errorf(
			"agentexec: waiting Interaction subtree change cannot continue from state %s",
			i.state,
		)
	}
	i.state = interactionWaitingSubtreeChangeContinued
	// Continue is the first operation that can advance the Process tree after the
	// Application durably opened the replacement product Segment.
	i.session.segmentClock.start()
	resumeCtx, cancelResume := context.WithTimeout(ctx, authoritativeProjectionTimeout)
	err := i.session.resumePausedProcesses(resumeCtx, i.paused)
	cancelResume()
	i.session.finishSubtreeContinuation(i, err)
	if err != nil {
		return fmt.Errorf("agentexec: continue applied waiting Interaction subtree: %w", err)
	}
	return nil
}

func (i *interactionWaitingSubtreeChange) Discard() error {
	if i == nil || i.session == nil || i.prepared == nil {
		return errors.New("agentexec: invalid waiting Interaction subtree change")
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangePrepared {
		return nil
	}
	i.stopExpirationLocked()
	if err := i.prepared.Discard(); err != nil {
		return fmt.Errorf("agentexec: discard waiting Interaction subtree: %w", err)
	}
	i.state = interactionWaitingSubtreeChangeDiscarded
	i.session.finishSubtreeDiscard(i)
	return nil
}

func (i *interactionWaitingSubtreeChange) armExpiration(ctx context.Context) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.state != interactionWaitingSubtreeChangePrepared {
		return
	}
	i.stopExpiry = context.AfterFunc(ctx, func() {
		_ = i.Discard()
	})
}

// stopExpirationLocked disarms the preparation lease while i.mu is held.
func (i *interactionWaitingSubtreeChange) stopExpirationLocked() {
	if i.stopExpiry != nil {
		i.stopExpiry()
		i.stopExpiry = nil
	}
}

func (i *interactionSession) beginSubtreeApplication(
	change *interactionWaitingSubtreeChange,
) error {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.finished || i.state.boundary != interactionBoundarySubtreePrepared ||
		i.state.subtreeChange != change {
		return runs.ErrExecutionClaimed
	}
	managedCalls := make([]*managedDelegateCall, len(change.canceled))
	for index, processID := range change.canceled {
		managed := i.state.delegateChildren[processID]
		if managed == nil {
			return fmt.Errorf("agentexec: canceled Interaction member %s has no Delegate binding", processID)
		}
		if i.state.delegateChildren[managed.childProcessID] != managed ||
			i.state.delegateCalls[managed.identity] != managed {
			return errors.New("agentexec: canceled Delegate binding changed before subtree application")
		}
		managedCalls[index] = managed
	}
	i.state.boundary = interactionBoundarySubtreeApplying
	change.retired = managedCalls
	return nil
}

func (i *interactionSession) commitSubtreeApplication(
	change *interactionWaitingSubtreeChange,
) {
	for _, managed := range change.retired {
		managed.mu.Lock()
		managed.parentToolFinished = true
		managed.assistantProjected = true
		managed.segmentProjected = true
		managed.mu.Unlock()
	}
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	i.state.waitingCheckpoint = change.checkpoint.Clone()
	for _, managed := range change.retired {
		delete(i.state.delegateChildren, managed.childProcessID)
		delete(i.state.delegateCalls, managed.identity)
		i.committedReplies.forget(managed.childProcessID)
	}
}

func (i *interactionSession) finishSubtreeApplication(
	change *interactionWaitingSubtreeChange,
	disposition runs.WaitingSubtreeDisposition,
	applyErr error,
) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.subtreeChange != change || i.state.boundary != interactionBoundarySubtreeApplying {
		return
	}
	if applyErr != nil {
		i.state.subtreeChange = nil
		i.state.boundary = interactionBoundarySubtreeRecovery
		return
	}
	if disposition == runs.WaitingSubtreeStaysWaiting {
		i.state.subtreeChange = nil
		i.state.boundary = interactionBoundaryWaiting
		return
	}
	i.state.boundary = interactionBoundarySubtreeApplied
}

func (i *interactionSession) finishSubtreeContinuation(
	change *interactionWaitingSubtreeChange,
	continuationErr error,
) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.subtreeChange != change || i.state.boundary != interactionBoundarySubtreeApplied {
		return
	}
	i.state.subtreeChange = nil
	if continuationErr != nil {
		i.state.boundary = interactionBoundarySubtreeRecovery
		return
	}
	i.state.boundary = interactionBoundaryInactive
	i.state.waitingCheckpoint = runs.ExecutorCheckpoint{}
}

func (i *interactionSession) finishSubtreeDiscard(change *interactionWaitingSubtreeChange) {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.subtreeChange != change || i.state.boundary != interactionBoundarySubtreePrepared {
		return
	}
	i.state.subtreeChange = nil
	i.state.boundary = interactionBoundaryWaiting
}

func (i *interactionSession) discardPreparedSubtree(ctx context.Context) error {
	for {
		i.state.mu.Lock()
		boundary := i.state.boundary
		preparedSignal := i.state.subtreePrepared
		change := i.state.subtreeChange
		i.state.mu.Unlock()
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
