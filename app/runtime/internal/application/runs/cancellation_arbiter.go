package runs

import (
	"context"
	"errors"
	"fmt"

	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/tool"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// childCancellation is the root Run-tree owner's one active child-cancel boundary.
// The owner admits either this boundary or whole-tree cancellation, never both.
type childCancellation struct {
	targetRunID    string
	parentRunID    string
	spawningItemID string
	reason         string
	targetRunIDs   map[string]struct{}
	rootSnapshot   rundomain.Run
	targetTerminal *rundomain.Run
	done           chan struct{}
	err            error
	finished       bool
}

func (owner *runTreeOwner) beginChildCancellation(
	plan cancellationPlan,
	reason string,
) (*childCancellation, error) {
	if owner == nil {
		return nil, errors.New("runs: child cancellation requires a live Run-tree owner")
	}
	if !plan.target.run.Lineage().IsChild() {
		return nil, fmt.Errorf("runs: cancellation target %q is not a child Run", plan.target.run.ID())
	}
	if plan.treeState != rundomain.Running {
		return nil, fmt.Errorf(
			"runs: live child cancellation requires a running tree, got %s",
			plan.treeState,
		)
	}
	if !plan.target.hasMember {
		return nil, fmt.Errorf(
			"runs: child cancellation target %q has no executor member binding",
			plan.target.run.ID(),
		)
	}
	targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
	for _, member := range plan.targetSubtree {
		if !member.run.State().IsTerminal() {
			targetRunIDs[member.run.ID()] = struct{}{}
		}
	}
	attempt := &childCancellation{
		targetRunID:    plan.target.run.ID(),
		parentRunID:    plan.target.run.Lineage().ParentRunID,
		spawningItemID: plan.target.run.Lineage().SpawnedByItemID,
		reason:         reason,
		targetRunIDs:   targetRunIDs,
		rootSnapshot:   plan.root.run,
		done:           make(chan struct{}),
	}

	owner.mu.Lock()
	defer owner.mu.Unlock()
	switch {
	case owner.cancelRequested:
		return nil, fmt.Errorf(
			"%w: root Run %q cancellation owns the tree",
			ErrSessionBusy,
			plan.root.run.ID(),
		)
	case owner.childCancel != nil:
		return nil, fmt.Errorf(
			"%w: child Run %q cancellation owns the tree",
			ErrSessionBusy,
			owner.childCancel.targetRunID,
		)
	case owner.terminalRuns != nil:
		if terminal, finished := owner.terminalRuns[attempt.targetRunID]; finished {
			return nil, fmt.Errorf(
				"%w: %q completed as %s",
				ErrRunFinished,
				attempt.targetRunID,
				terminal.State(),
			)
		}
	}
	owner.childCancel = attempt
	return attempt, nil
}

func (owner *runTreeOwner) abortChildCancellation(attempt *childCancellation, err error) {
	if owner == nil || attempt == nil {
		return
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.childCancel != attempt || attempt.finished {
		return
	}
	attempt.err = err
	owner.finishChildCancellationLocked(attempt)
}

func (owner *runTreeOwner) finishChildCancellationLocked(attempt *childCancellation) {
	if attempt == nil || attempt.finished {
		return
	}
	attempt.finished = true
	if owner.childCancel == attempt {
		owner.childCancel = nil
	}
	close(attempt.done)
}

func (owner *runTreeOwner) classifyChildCancellationTool(
	parentRunID string,
	itemID string,
	event ToolCallFinished,
) ToolCallFinished {
	if owner == nil {
		return event
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	attempt := owner.childCancel
	if attempt == nil ||
		attempt.targetTerminal == nil ||
		attempt.parentRunID != parentRunID ||
		attempt.spawningItemID != itemID {
		return event
	}
	classified := event
	classified.Failure = &tool.Failure{
		Kind:   tool.FailureChildRunCanceled,
		Detail: attempt.reason,
	}
	return classified
}

func (owner *runTreeOwner) recordChildCancellationItem(parentRunID string, item transcript.Item) {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	attempt := owner.childCancel
	if attempt == nil ||
		attempt.targetTerminal == nil ||
		attempt.parentRunID != parentRunID ||
		attempt.spawningItemID != item.ID() {
		return
	}
	failure, failed := item.Failure()
	if !failed || failure.Kind != tool.FailureChildRunCanceled {
		attempt.err = fmt.Errorf(
			"runs: canceled child Run %q parent item %q committed without child_run_canceled",
			attempt.targetRunID,
			item.ID(),
		)
	} else if item.Status() != transcript.ItemIncomplete {
		attempt.err = fmt.Errorf(
			"runs: canceled child Run %q parent item %q committed in status %s",
			attempt.targetRunID,
			item.ID(),
			item.Status(),
		)
	}
	owner.finishChildCancellationLocked(attempt)
}

func (owner *runTreeOwner) waitChildCancellation(
	ctx context.Context,
	attempt *childCancellation,
) (rundomain.Run, rundomain.Run, error) {
	if owner == nil || attempt == nil {
		return rundomain.Run{}, rundomain.Run{}, errors.New("runs: missing child cancellation attempt")
	}
	select {
	case <-attempt.done:
	case <-owner.done:
		select {
		case <-attempt.done:
		default:
			if owner.completionErr != nil {
				return rundomain.Run{}, rundomain.Run{}, owner.completionErr
			}
			return rundomain.Run{}, rundomain.Run{}, fmt.Errorf(
				"runs: root segment ended before child Run %q cancellation committed its parent result",
				attempt.targetRunID,
			)
		}
	case <-ctx.Done():
		return rundomain.Run{}, rundomain.Run{}, ctx.Err()
	}
	if attempt.err != nil {
		return rundomain.Run{}, rundomain.Run{}, attempt.err
	}
	if attempt.targetTerminal == nil {
		return rundomain.Run{}, rundomain.Run{}, fmt.Errorf(
			"runs: child Run %q cancellation completed without a terminal snapshot",
			attempt.targetRunID,
		)
	}
	return *attempt.targetTerminal, attempt.rootSnapshot, nil
}

// recordTerminalRun retains the exact Run snapshot whose terminal transaction
// just committed. Root cancellation reads the root after joining done; child
// cancellation arms its parent-result classification only after the target's
// canceled terminal is durable.
func (owner *runTreeOwner) recordTerminalRun(run rundomain.Run) {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.terminalRuns == nil {
		owner.terminalRuns = make(map[string]rundomain.Run)
	}
	if _, duplicate := owner.terminalRuns[run.ID()]; duplicate {
		panic(fmt.Sprintf("runs: Run %q committed more than one terminal snapshot", run.ID()))
	}
	owner.terminalRuns[run.ID()] = run
	if run.Lineage().IsRoot() {
		if owner.terminalRun != nil {
			panic("runs: live segment committed more than one root terminal snapshot")
		}
		owner.terminalRun = &run
	}
	attempt := owner.childCancel
	if attempt == nil || run.ID() != attempt.targetRunID {
		return
	}
	outcome, hasOutcome := run.Outcome()
	if run.State() != rundomain.Canceled || !hasOutcome || outcome != rundomain.OutcomeCanceled {
		attempt.err = fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			run.ID(),
			run.State(),
		)
		owner.finishChildCancellationLocked(attempt)
		return
	}
	terminal := run
	attempt.targetTerminal = &terminal
}

// interruptBoundary records the only two durable facts cancellation needs:
// whether publication committed, and whether one publication is currently
// owned. Both fields are guarded by runTreeOwner.mu.
type interruptBoundary struct {
	committed bool
	active    *interruptCommit
}

// interruptCommit is the one cancellable interrupt publication a run may own.
// A nil pointer means there is no commit to interrupt or join.
type interruptCommit struct {
	done   chan struct{}
	cancel context.CancelFunc
}

// requestCancel linearizes cancellation with child cancellation and interrupt
// publication. Once it returns successfully, no child cancellation or new
// interrupt can own this tree.
func (owner *runTreeOwner) requestCancel(
	ctx context.Context,
	reason string,
	requestExecutor func(context.Context) error,
) (interruptCommitted bool, err error) {
	if owner == nil {
		return false, nil
	}
	for {
		owner.mu.Lock()
		if owner.childCancel != nil {
			targetRunID := owner.childCancel.targetRunID
			owner.mu.Unlock()
			return false, fmt.Errorf(
				"%w: child Run %q cancellation owns the tree",
				ErrSessionBusy,
				targetRunID,
			)
		}
		if owner.cancelRequested {
			owner.mu.Unlock()
			return false, fmt.Errorf(
				"%w: root Run cancellation already owns the tree",
				ErrSessionBusy,
			)
		}
		activation := owner.activation
		if activation.done != nil && activation.started && !activation.finished {
			owner.mu.Unlock()
			select {
			case <-activation.done:
				continue
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
		if activation.done != nil && activation.finished && activation.err != nil {
			owner.mu.Unlock()
			return false, fmt.Errorf(
				"%w: segment activation failed: %v",
				ErrRunFinished,
				activation.err,
			)
		}
		owner.cancelRequested = true
		owner.cancelReason = reason
		inflight := owner.interrupt.active
		owner.mu.Unlock()
		if requestExecutor == nil {
			owner.abortRootCancellation(reason)
			return false, errors.New("runs: root executor cancellation request is unavailable")
		}
		if err := requestExecutor(ctx); err != nil {
			owner.abortRootCancellation(reason)
			return false, err
		}
		if inflight != nil {
			inflight.cancel()
		}
		if inflight != nil {
			select {
			case <-inflight.done:
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
		owner.mu.Lock()
		committed := owner.interrupt.committed
		owner.mu.Unlock()
		return committed, nil
	}
}

func (owner *runTreeOwner) abortRootCancellation(reason string) {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.cancelRequested && owner.cancelReason == reason {
		owner.cancelRequested = false
		owner.cancelReason = ""
	}
}

// commitInterrupt reserves the interrupt boundary, runs its context-bounded
// durable commit and publication without holding mu, then releases waiting
// cancellation. committed=false means cancellation won before the reservation
// or the commit failed.
func (owner *runTreeOwner) commitInterrupt(ctx context.Context, commit func(context.Context) error) (committed bool, err error) {
	if owner == nil {
		return false, errors.New("runs: missing live Run-tree owner")
	}
	commitCtx, cancelCommit := context.WithTimeout(ctx, runCleanupTimeout)
	owner.mu.Lock()
	if owner.cancelRequested {
		owner.mu.Unlock()
		cancelCommit()
		return false, nil
	}
	if owner.interrupt.committed || owner.interrupt.active != nil {
		owner.mu.Unlock()
		cancelCommit()
		return false, errors.New("runs: interrupt boundary already resolved")
	}
	inflight := &interruptCommit{done: make(chan struct{}), cancel: cancelCommit}
	owner.interrupt.active = inflight
	owner.mu.Unlock()

	err = commit(commitCtx)
	cancelCommit()
	owner.mu.Lock()
	if err == nil {
		owner.interrupt.committed = true
	}
	close(inflight.done)
	owner.interrupt.active = nil
	owner.mu.Unlock()
	if err != nil {
		return false, err
	}
	return true, nil
}

// CancelReason returns the recorded human cancel reason. It is late-bound on
// purpose because cancellation can arrive after the segment starts.
func (owner *runTreeOwner) CancelReason() string {
	if owner == nil {
		return ""
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.cancelReason
}

// CancelReasonFor returns the reason owned by runID's winning cancellation
// plan. A child operation applies its reason only to the target subtree; the
// root reason remains independent for a later whole-tree cancellation.
func (owner *runTreeOwner) CancelReasonFor(runID string) string {
	if owner == nil {
		return ""
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.childCancel != nil {
		if _, targeted := owner.childCancel.targetRunIDs[runID]; targeted {
			return owner.childCancel.reason
		}
	}
	return owner.cancelReason
}
