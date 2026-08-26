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

func (r *runTreeOwner) beginChildCancellation(
	plan cancellationPlan,
	reason string,
) (*childCancellation, error) {
	if r == nil {
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

	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case r.cancelRequested:
		return nil, fmt.Errorf(
			"%w: root Run %q cancellation owns the tree",
			ErrSessionBusy,
			plan.root.run.ID(),
		)
	case r.childCancel != nil:
		return nil, fmt.Errorf(
			"%w: child Run %q cancellation owns the tree",
			ErrSessionBusy,
			r.childCancel.targetRunID,
		)
	case r.terminalRuns != nil:
		if terminal, finished := r.terminalRuns[attempt.targetRunID]; finished {
			return nil, fmt.Errorf(
				"%w: %q completed as %s",
				ErrRunFinished,
				attempt.targetRunID,
				terminal.State(),
			)
		}
	}
	r.childCancel = attempt
	return attempt, nil
}

func (r *runTreeOwner) abortChildCancellation(attempt *childCancellation, err error) {
	if r == nil || attempt == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.childCancel != attempt || attempt.finished {
		return
	}
	attempt.err = err
	r.finishChildCancellationLocked(attempt)
}

func (r *runTreeOwner) finishChildCancellationLocked(attempt *childCancellation) {
	if attempt == nil || attempt.finished {
		return
	}
	attempt.finished = true
	if r.childCancel == attempt {
		r.childCancel = nil
	}
	close(attempt.done)
}

func (r *runTreeOwner) classifyChildCancellationTool(
	parentRunID string,
	itemID string,
	event ToolCallFinished,
) ToolCallFinished {
	if r == nil {
		return event
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.childCancel
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

func (r *runTreeOwner) recordChildCancellationItem(parentRunID string, item transcript.Item) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	attempt := r.childCancel
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
	r.finishChildCancellationLocked(attempt)
}

func (r *runTreeOwner) waitChildCancellation(
	ctx context.Context,
	attempt *childCancellation,
) (rundomain.Run, rundomain.Run, error) {
	if r == nil || attempt == nil {
		return rundomain.Run{}, rundomain.Run{}, errors.New("runs: missing child cancellation attempt")
	}
	select {
	case <-attempt.done:
	case <-r.done:
		select {
		case <-attempt.done:
		default:
			if r.completionErr != nil {
				return rundomain.Run{}, rundomain.Run{}, r.completionErr
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
func (r *runTreeOwner) recordTerminalRun(run rundomain.Run) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.terminalRuns == nil {
		r.terminalRuns = make(map[string]rundomain.Run)
	}
	if _, duplicate := r.terminalRuns[run.ID()]; duplicate {
		panic(fmt.Sprintf("runs: Run %q committed more than one terminal snapshot", run.ID()))
	}
	r.terminalRuns[run.ID()] = run
	if run.Lineage().IsRoot() {
		if r.terminalRun != nil {
			panic("runs: live segment committed more than one root terminal snapshot")
		}
		r.terminalRun = &run
	}
	attempt := r.childCancel
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
		r.finishChildCancellationLocked(attempt)
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
func (r *runTreeOwner) requestCancel(
	ctx context.Context,
	reason string,
	requestExecutor func(context.Context) error,
) (interruptCommitted bool, err error) {
	if r == nil {
		return false, nil
	}
	for {
		r.mu.Lock()
		if r.childCancel != nil {
			targetRunID := r.childCancel.targetRunID
			r.mu.Unlock()
			return false, fmt.Errorf(
				"%w: child Run %q cancellation owns the tree",
				ErrSessionBusy,
				targetRunID,
			)
		}
		if r.cancelRequested {
			r.mu.Unlock()
			return false, fmt.Errorf(
				"%w: root Run cancellation already owns the tree",
				ErrSessionBusy,
			)
		}
		activation := r.activation
		if activation.done != nil && activation.started && !activation.finished {
			r.mu.Unlock()
			select {
			case <-activation.done:
				continue
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
		if activation.done != nil && activation.finished && activation.err != nil {
			r.mu.Unlock()
			return false, fmt.Errorf(
				"%w: segment activation failed: %v",
				ErrRunFinished,
				activation.err,
			)
		}
		r.cancelRequested = true
		r.cancelReason = reason
		inflight := r.interrupt.active
		r.mu.Unlock()
		if requestExecutor == nil {
			r.abortRootCancellation(reason)
			return false, errors.New("runs: root executor cancellation request is unavailable")
		}
		if err := requestExecutor(ctx); err != nil {
			r.abortRootCancellation(reason)
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
		r.mu.Lock()
		committed := r.interrupt.committed
		r.mu.Unlock()
		return committed, nil
	}
}

func (r *runTreeOwner) abortRootCancellation(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancelRequested && r.cancelReason == reason {
		r.cancelRequested = false
		r.cancelReason = ""
	}
}

// commitInterrupt reserves the interrupt boundary, runs its context-bounded
// durable commit and publication without holding mu, then releases waiting
// cancellation. committed=false means cancellation won before the reservation
// or the commit failed.
func (r *runTreeOwner) commitInterrupt(ctx context.Context, commit func(context.Context) error) (committed bool, err error) {
	if r == nil {
		return false, errors.New("runs: missing live Run-tree owner")
	}
	commitCtx, cancelCommit := context.WithTimeout(ctx, runCleanupTimeout)
	r.mu.Lock()
	if r.cancelRequested {
		r.mu.Unlock()
		cancelCommit()
		return false, nil
	}
	if r.interrupt.committed || r.interrupt.active != nil {
		r.mu.Unlock()
		cancelCommit()
		return false, errors.New("runs: interrupt boundary already resolved")
	}
	inflight := &interruptCommit{done: make(chan struct{}), cancel: cancelCommit}
	r.interrupt.active = inflight
	r.mu.Unlock()

	err = commit(commitCtx)
	cancelCommit()
	r.mu.Lock()
	if err == nil {
		r.interrupt.committed = true
	}
	close(inflight.done)
	r.interrupt.active = nil
	r.mu.Unlock()
	if err != nil {
		return false, err
	}
	return true, nil
}

// CancelReason returns the recorded human cancel reason. It is late-bound on
// purpose because cancellation can arrive after the segment starts.
func (r *runTreeOwner) CancelReason() string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cancelReason
}

// CancelReasonFor returns the reason owned by runID's winning cancellation
// plan. A child operation applies its reason only to the target subtree; the
// root reason remains independent for a later whole-tree cancellation.
func (r *runTreeOwner) CancelReasonFor(runID string) string {
	if r == nil {
		return ""
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.childCancel != nil {
		if _, targeted := r.childCancel.targetRunIDs[runID]; targeted {
			return r.childCancel.reason
		}
	}
	return r.cancelReason
}
