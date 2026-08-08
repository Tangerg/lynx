package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

type waitingCancellationTransformation struct {
	terminalRuns   []transcript.Run
	terminalItems  []ItemReplacement
	parentItem     ItemReplacement
	remaining      *Pending
	continuation   *treeContinuation
	checkpoint     ExecutorCheckpoint
	root           transcript.Run
	targetRunID    string
	canceledRunIDs []string
}

func prepareWaitingCancellationTransformation(
	plan cancellationPlan,
	reason string,
	finishedAt time.Time,
	prepared PreparedWaitingSubtreeCancellation,
) (waitingCancellationTransformation, error) {
	switch {
	case plan.treeState != rundomain.Waiting:
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: waiting cancellation plan is %s",
			plan.treeState,
		)
	case !plan.target.run.Lineage().IsChild():
		return waitingCancellationTransformation{}, errors.New("runs: waiting cancellation target is not a child Run")
	case !plan.hasPending:
		return waitingCancellationTransformation{}, errors.New("runs: waiting cancellation plan has no pending set")
	case !plan.hasSpawningItem:
		return waitingCancellationTransformation{}, errors.New("runs: waiting cancellation plan has no spawning Item")
	case prepared.Mutation == nil:
		return waitingCancellationTransformation{}, errors.New("runs: prepared waiting subtree cancellation has no mutation lease")
	case finishedAt.IsZero():
		return waitingCancellationTransformation{}, errors.New("runs: waiting cancellation finish time is required")
	}
	rootContinuation, ok := plan.pending.RootContinuation()
	if !ok {
		return waitingCancellationTransformation{}, errors.New("runs: waiting cancellation Pending has no root continuation")
	}
	if err := prepared.Checkpoint.ValidateOwnership(rootContinuation.MemberID, plan.pending.SessionID); err != nil {
		return waitingCancellationTransformation{}, fmt.Errorf("runs: invalid prepared waiting subtree checkpoint ownership: %w", err)
	}
	if prepared.Checkpoint.Scope.GoalLeaseID != plan.pending.GoalLeaseID {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: prepared waiting subtree checkpoint goal lease %q does not match Pending %q: %w",
			prepared.Checkpoint.Scope.GoalLeaseID,
			plan.pending.GoalLeaseID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if prepared.Checkpoint.ModelSelection != rootContinuation.ModelSelection {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: prepared waiting subtree checkpoint model %q/%q does not match root continuation %q/%q: %w",
			prepared.Checkpoint.ModelSelection.Provider(),
			prepared.Checkpoint.ModelSelection.Model(),
			rootContinuation.ModelSelection.Provider(),
			rootContinuation.ModelSelection.Model(),
			ErrInvalidExecutorCheckpoint,
		)
	}
	if prepared.Checkpoint.Limits != rootContinuation.Limits {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: prepared waiting subtree checkpoint limits %+v do not match root continuation %+v: %w",
			prepared.Checkpoint.Limits,
			rootContinuation.Limits,
			ErrInvalidExecutorCheckpoint,
		)
	}

	canceledProcesses := prepared.CanceledMemberIDs
	canceledMemberSet := make(map[string]struct{}, len(canceledProcesses))
	for _, memberID := range canceledProcesses {
		if memberID == "" {
			return waitingCancellationTransformation{}, errors.New(
				"runs: prepared waiting cancellation returned an empty member id",
			)
		}
		if _, duplicate := canceledMemberSet[memberID]; duplicate {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared waiting cancellation repeated member %q",
				memberID,
			)
		}
		canceledMemberSet[memberID] = struct{}{}
	}

	expectedProcesses := make(map[string]struct{})
	var terminalRuns []transcript.Run
	var canceledRunIDs []string
	for _, member := range plan.targetSubtree {
		if member.run.State.IsTerminal() {
			continue
		}
		if !member.hasMember {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: waiting cancellation target Run %q has no executor member",
				member.run.ID,
			)
		}
		expectedProcesses[member.memberID] = struct{}{}
		terminalRuns = append(terminalRuns, canceledWaitingRun(member.run, reason, finishedAt))
		canceledRunIDs = append(canceledRunIDs, member.run.ID)
	}
	if len(canceledMemberSet) != len(expectedProcesses) {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: prepared waiting cancellation removed %d members, Run subtree requires %d",
			len(canceledMemberSet),
			len(expectedProcesses),
		)
	}
	for memberID := range expectedProcesses {
		if _, canceled := canceledMemberSet[memberID]; !canceled {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared waiting cancellation did not remove member %q",
				memberID,
			)
		}
	}

	problem := transcript.Problem{
		Kind:   transcript.ChildRunCanceledProblem,
		Scope:  transcript.ToolProblem,
		Detail: reason,
	}
	parentItem := plan.spawningItem
	replacement := parentItem
	replacement.Status = transcript.ItemIncomplete
	replacement.Error = &problem
	terminalItems := make([]ItemReplacement, 0, len(plan.targetInterruptItems))
	for _, item := range plan.targetInterruptItems {
		settled := item
		settled.Status = transcript.ItemIncomplete
		if settled.Kind == transcript.ToolCall {
			settled.FinishedAt = finishedAt.UTC()
			settled.Error = &transcript.Problem{
				Kind:   transcript.ToolFailedProblem,
				Scope:  transcript.ToolProblem,
				Detail: reason,
			}
		}
		terminalItems = append(terminalItems, ItemReplacement{
			Expected:    item,
			Replacement: settled,
		})
	}

	continuations := make([]Continuation, 0, len(plan.survivingTree))
	parentToolMoved := false
	for _, continuation := range plan.pending.Continuations {
		if _, canceled := canceledMemberSet[continuation.MemberID]; canceled {
			continue
		}
		clone := continuation
		clone.DrainedTools = slices.Clone(continuation.DrainedTools)
		clone.CommittedTools = slices.Clone(continuation.CommittedTools)
		if continuation.RunID == plan.target.run.ParentRunID {
			var matches []DrainedTool
			clone.DrainedTools = slices.DeleteFunc(clone.DrainedTools, func(tool DrainedTool) bool {
				if tool.ItemID != parentItem.ID {
					return false
				}
				matches = append(matches, tool)
				return true
			})
			if len(matches) != 1 {
				return waitingCancellationTransformation{}, fmt.Errorf(
					"runs: parent Run %q continuation has %d drained tools for spawning Item %q",
					continuation.RunID,
					len(matches),
					parentItem.ID,
				)
			}
			tool := matches[0]
			if tool.Name != parentItem.Tool.Name ||
				tool.Arguments != parentItem.Tool.Arguments.Canonical() {
				return waitingCancellationTransformation{}, fmt.Errorf(
					"runs: spawning Item %q differs from its drained tool identity",
					parentItem.ID,
				)
			}
			clone.CommittedTools = append(clone.CommittedTools, CommittedTool{
				ItemID:    tool.ItemID,
				CallID:    tool.CallID,
				Name:      tool.Name,
				Arguments: tool.Arguments,
				Problem:   problem,
			})
			parentToolMoved = true
		}
		continuations = append(continuations, clone)
	}
	if !parentToolMoved {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: waiting cancellation did not settle spawning Item %q",
			parentItem.ID,
		)
	}

	oldBindingByKey := make(map[string]int, len(plan.pending.Bindings))
	for index, binding := range plan.pending.Bindings {
		oldBindingByKey[inputRequestIdentity(binding.MemberID, binding.RequestID)] = index
	}
	survivingRunByProcess := make(map[string]string, len(continuations))
	for _, continuation := range continuations {
		survivingRunByProcess[continuation.MemberID] = continuation.RunID
	}
	pendingInterruptions := prepared.PendingInterruptions
	remainingInterrupts := make([]transcript.Interrupt, 0, len(pendingInterruptions))
	remainingBindings := make([]InterruptBinding, 0, len(pendingInterruptions))
	keptBindings := make(map[int]struct{}, len(pendingInterruptions))
	for _, boundary := range pendingInterruptions {
		if err := boundary.Interrupt.Validate(); err != nil {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared member %q input request %q: %w",
				boundary.MemberID,
				boundary.RequestID,
				err,
			)
		}
		index, exists := oldBindingByKey[inputRequestIdentity(boundary.MemberID, boundary.RequestID)]
		if !exists {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared member %q input request %q was absent from the durable pending set",
				boundary.MemberID,
				boundary.RequestID,
			)
		}
		if _, duplicate := keptBindings[index]; duplicate {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared member %q repeated input request %q",
				boundary.MemberID,
				boundary.RequestID,
			)
		}
		binding := plan.pending.Bindings[index]
		interrupt := plan.pending.Interrupts[index]
		runID, survives := survivingRunByProcess[binding.MemberID]
		if !survives || interrupt.RunID != runID {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared input request %q belongs to removed member %q",
				binding.RequestID,
				binding.MemberID,
			)
		}
		if interrupt.Kind != boundary.Interrupt.Kind {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared input request %q changed interrupt kind from %s to %s",
				binding.RequestID,
				interrupt.Kind,
				boundary.Interrupt.Kind,
			)
		}
		keptBindings[index] = struct{}{}
		remainingInterrupts = append(remainingInterrupts, interrupt)
		remainingBindings = append(remainingBindings, binding)
	}
	for index, binding := range plan.pending.Bindings {
		if _, kept := keptBindings[index]; kept {
			continue
		}
		if _, canceled := canceledMemberSet[binding.MemberID]; !canceled {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared cancellation dropped surviving member %q input request %q",
				binding.MemberID,
				binding.RequestID,
			)
		}
	}

	continuation := &treeContinuation{
		rootRunID:     plan.pending.RootRunID,
		sessionID:     plan.pending.SessionID,
		executorID:    plan.pending.ExecutorID,
		goalLeaseID:   plan.pending.GoalLeaseID,
		interrupts:    slices.Clone(remainingInterrupts),
		continuations: slices.Clone(continuations),
		capabilities:  plan.pending.Capabilities,
	}
	if err := continuation.validate(); err != nil {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: waiting cancellation continuation: %w",
			err,
		)
	}

	var remaining *Pending
	if len(remainingInterrupts) > 0 {
		reduced := plan.pending
		reduced.Interrupts = remainingInterrupts
		reduced.Bindings = remainingBindings
		reduced.Continuations = continuations
		if err := reduced.Validate(); err != nil {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: reduced waiting cancellation pending set: %w",
				err,
			)
		}
		remaining = &reduced
	}

	return waitingCancellationTransformation{
		terminalRuns:   terminalRuns,
		terminalItems:  terminalItems,
		parentItem:     ItemReplacement{Expected: parentItem, Replacement: replacement},
		remaining:      remaining,
		continuation:   continuation,
		checkpoint:     prepared.Checkpoint.Clone(),
		root:           plan.root.run,
		targetRunID:    plan.target.run.ID,
		canceledRunIDs: canceledRunIDs,
	}, nil
}

func canceledWaitingRun(run transcript.Run, reason string, finishedAt time.Time) transcript.Run {
	outcome := rundomain.OutcomeCanceled
	run.State = rundomain.Canceled
	run.ActiveSegmentID = ""
	run.Outcome = &outcome
	run.Detail = reason
	run.Error = nil
	run.Interrupts = nil
	run.FinishedAt = finishedAt.UTC()
	run.UpdatedAt = finishedAt.UTC()
	run.MessageMark = transcript.UnknownMessageMark
	return run
}

func inputRequestIdentity(memberID, requestID string) string {
	return memberID + "\x00" + requestID
}

func (transformation waitingCancellationTransformation) durableCommit(
	expected Pending,
) WaitingSubtreeCancellationCommit {
	return WaitingSubtreeCancellationCommit{
		RootRunID:        expected.RootRunID,
		TargetRunID:      transformation.targetRunID,
		SessionID:        expected.SessionID,
		RootRun:          transformation.root,
		ExpectedPending:  expected,
		RemainingPending: transformation.remaining,
		Checkpoint:       transformation.checkpoint,
		TerminalRuns:     slices.Clone(transformation.terminalRuns),
		TerminalItems:    slices.Clone(transformation.terminalItems),
		ParentItem:       transformation.parentItem,
	}
}
