package runs

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// childCancellation is the root handle's one active child-cancel boundary.
// The handle admits either this boundary or whole-tree cancellation, never both.
type childCancellation struct {
	targetRunID    string
	parentRunID    string
	spawningItemID string
	reason         string
	targetRunIDs   map[string]struct{}
	rootSnapshot   transcript.Run
	targetTerminal *transcript.Run
	done           chan struct{}
	err            error
	finished       bool
}

func (h *handle) beginChildCancellation(
	plan cancellationPlan,
	reason string,
) (*childCancellation, error) {
	if h == nil {
		return nil, errors.New("runs: child cancellation requires a live root handle")
	}
	if !plan.target.run.Lineage().IsChild() {
		return nil, fmt.Errorf("runs: cancellation target %q is not a child Run", plan.target.run.ID)
	}
	if plan.treeState != execution.Running {
		return nil, fmt.Errorf(
			"runs: live child cancellation requires a running tree, got %s",
			plan.treeState,
		)
	}
	if !plan.target.hasProcess {
		return nil, fmt.Errorf(
			"runs: child cancellation target %q has no executor process binding",
			plan.target.run.ID,
		)
	}
	targetRunIDs := make(map[string]struct{}, len(plan.targetSubtree))
	for _, member := range plan.targetSubtree {
		if !member.run.State.IsTerminal() {
			targetRunIDs[member.run.ID] = struct{}{}
		}
	}
	attempt := &childCancellation{
		targetRunID:    plan.target.run.ID,
		parentRunID:    plan.target.run.ParentRunID,
		spawningItemID: plan.target.run.SpawnedByItemID,
		reason:         reason,
		targetRunIDs:   targetRunIDs,
		rootSnapshot:   plan.root.run,
		done:           make(chan struct{}),
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case h.cancelRequested:
		return nil, fmt.Errorf(
			"%w: root Run %q cancellation owns the tree",
			ErrSessionBusy,
			plan.root.run.ID,
		)
	case h.childCancel != nil:
		return nil, fmt.Errorf(
			"%w: child Run %q cancellation owns the tree",
			ErrSessionBusy,
			h.childCancel.targetRunID,
		)
	case h.terminalRuns != nil:
		if terminal, finished := h.terminalRuns[attempt.targetRunID]; finished {
			return nil, fmt.Errorf(
				"%w: %q completed as %s",
				ErrRunFinished,
				attempt.targetRunID,
				terminal.State,
			)
		}
	}
	h.childCancel = attempt
	return attempt, nil
}

func (h *handle) abortChildCancellation(attempt *childCancellation, err error) {
	if h == nil || attempt == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.childCancel != attempt || attempt.finished {
		return
	}
	attempt.err = err
	h.finishChildCancellationLocked(attempt)
}

func (h *handle) finishChildCancellationLocked(attempt *childCancellation) {
	if attempt == nil || attempt.finished {
		return
	}
	attempt.finished = true
	if h.childCancel == attempt {
		h.childCancel = nil
	}
	close(attempt.done)
}

func (h *handle) classifyChildCancellationTool(
	parentRunID string,
	itemID string,
	event ToolCallEnd,
) ToolCallEnd {
	if h == nil {
		return event
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	attempt := h.childCancel
	if attempt == nil ||
		attempt.targetTerminal == nil ||
		attempt.parentRunID != parentRunID ||
		attempt.spawningItemID != itemID {
		return event
	}
	classified := event
	classified.Problem = &transcript.Problem{
		Kind:   transcript.ChildRunCanceledProblem,
		Scope:  transcript.ToolProblem,
		Detail: attempt.reason,
	}
	return classified
}

func (h *handle) recordChildCancellationItem(parentRunID string, item transcript.Item) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	attempt := h.childCancel
	if attempt == nil ||
		attempt.targetTerminal == nil ||
		attempt.parentRunID != parentRunID ||
		attempt.spawningItemID != item.ID {
		return
	}
	if item.Error == nil || item.Error.Kind != transcript.ChildRunCanceledProblem {
		attempt.err = fmt.Errorf(
			"runs: canceled child Run %q parent item %q committed without child_run_canceled",
			attempt.targetRunID,
			item.ID,
		)
	} else if item.Status != transcript.ItemIncomplete {
		attempt.err = fmt.Errorf(
			"runs: canceled child Run %q parent item %q committed in status %d",
			attempt.targetRunID,
			item.ID,
			item.Status,
		)
	}
	h.finishChildCancellationLocked(attempt)
}

func (h *handle) waitChildCancellation(
	ctx context.Context,
	attempt *childCancellation,
) (transcript.Run, transcript.Run, error) {
	if h == nil || attempt == nil {
		return transcript.Run{}, transcript.Run{}, errors.New("runs: missing child cancellation attempt")
	}
	select {
	case <-attempt.done:
	case <-h.done:
		select {
		case <-attempt.done:
		default:
			if h.completionErr != nil {
				return transcript.Run{}, transcript.Run{}, h.completionErr
			}
			return transcript.Run{}, transcript.Run{}, fmt.Errorf(
				"runs: root segment ended before child Run %q cancellation committed its parent result",
				attempt.targetRunID,
			)
		}
	case <-ctx.Done():
		return transcript.Run{}, transcript.Run{}, ctx.Err()
	}
	if attempt.err != nil {
		return transcript.Run{}, transcript.Run{}, attempt.err
	}
	if attempt.targetTerminal == nil {
		return transcript.Run{}, transcript.Run{}, fmt.Errorf(
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
func (h *handle) recordTerminalRun(run transcript.Run) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.terminalRuns == nil {
		h.terminalRuns = make(map[string]transcript.Run)
	}
	if _, duplicate := h.terminalRuns[run.ID]; duplicate {
		panic(fmt.Sprintf("runs: Run %q committed more than one terminal snapshot", run.ID))
	}
	h.terminalRuns[run.ID] = run
	if run.Lineage().IsRoot() {
		if h.terminalRun != nil {
			panic("runs: live segment committed more than one root terminal snapshot")
		}
		h.terminalRun = &run
	}
	attempt := h.childCancel
	if attempt == nil || run.ID != attempt.targetRunID {
		return
	}
	if run.State != execution.Canceled ||
		run.Outcome == nil ||
		*run.Outcome != execution.OutcomeCanceled {
		attempt.err = fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			run.ID,
			run.State,
		)
		h.finishChildCancellationLocked(attempt)
		return
	}
	terminal := run
	attempt.targetTerminal = &terminal
}

// interruptBoundary records the only two durable facts cancellation needs:
// whether publication committed, and whether one publication is currently
// owned. Both fields are guarded by handle.mu.
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
func (h *handle) requestCancel(ctx context.Context, reason string) (interruptCommitted bool, err error) {
	if h == nil {
		return false, nil
	}
	h.mu.Lock()
	if h.childCancel != nil {
		targetRunID := h.childCancel.targetRunID
		h.mu.Unlock()
		return false, fmt.Errorf(
			"%w: child Run %q cancellation owns the tree",
			ErrSessionBusy,
			targetRunID,
		)
	}
	h.cancelRequested = true
	h.cancelReason = reason
	cancelRun := h.cancel
	inflight := h.interrupt.active
	h.mu.Unlock()
	if cancelRun != nil {
		cancelRun()
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
	h.mu.Lock()
	committed := h.interrupt.committed
	h.mu.Unlock()
	return committed, nil
}

// commitInterrupt reserves the interrupt boundary, runs its context-bounded
// durable commit and publication without holding mu, then releases waiting
// cancellation. committed=false means cancellation won before the reservation
// or the commit failed.
func (h *handle) commitInterrupt(ctx context.Context, commit func(context.Context) error) (committed bool, err error) {
	if h == nil {
		return false, errors.New("runs: missing live run handle")
	}
	commitCtx, cancelCommit := context.WithTimeout(ctx, runCleanupTimeout)
	h.mu.Lock()
	if h.cancelRequested {
		h.mu.Unlock()
		cancelCommit()
		return false, nil
	}
	if h.interrupt.committed || h.interrupt.active != nil {
		h.mu.Unlock()
		cancelCommit()
		return false, errors.New("runs: interrupt boundary already resolved")
	}
	inflight := &interruptCommit{done: make(chan struct{}), cancel: cancelCommit}
	h.interrupt.active = inflight
	h.mu.Unlock()

	err = commit(commitCtx)
	cancelCommit()
	h.mu.Lock()
	if err == nil {
		h.interrupt.committed = true
	}
	close(inflight.done)
	h.interrupt.active = nil
	h.mu.Unlock()
	if err != nil {
		return false, err
	}
	return true, nil
}

// CancelReason returns the recorded human cancel reason. It is late-bound on
// purpose because cancellation can arrive after the segment starts.
func (h *handle) CancelReason() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cancelReason
}

// CancelReasonFor returns the reason owned by runID's winning cancellation
// plan. A child operation applies its reason only to the target subtree; the
// root reason remains independent for a later whole-tree cancellation.
func (h *handle) CancelReasonFor(runID string) string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.childCancel != nil {
		if _, targeted := h.childCancel.targetRunIDs[runID]; targeted {
			return h.childCancel.reason
		}
	}
	return h.cancelReason
}
