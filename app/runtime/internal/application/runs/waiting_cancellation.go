package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

type waitingCancellationTransformation struct {
	terminalRuns   []transcript.Run
	terminalItems  []ItemReplacement
	parentItem     ItemReplacement
	remaining      *interrupts.Pending
	continuation   *treeContinuation
	checkpoint     ProcessCheckpointWrite
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
	case plan.treeState != execution.Interrupted:
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
	case prepared == nil:
		return waitingCancellationTransformation{}, errors.New("runs: prepared waiting subtree cancellation is required")
	case finishedAt.IsZero():
		return waitingCancellationTransformation{}, errors.New("runs: waiting cancellation finish time is required")
	}

	canceledProcesses := prepared.CanceledProcessIDs()
	canceledProcessSet := make(map[string]struct{}, len(canceledProcesses))
	for _, processID := range canceledProcesses {
		if processID == "" {
			return waitingCancellationTransformation{}, errors.New(
				"runs: prepared waiting cancellation returned an empty process id",
			)
		}
		if _, duplicate := canceledProcessSet[processID]; duplicate {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared waiting cancellation repeated process %q",
				processID,
			)
		}
		canceledProcessSet[processID] = struct{}{}
	}

	expectedProcesses := make(map[string]struct{})
	var terminalRuns []transcript.Run
	var canceledRunIDs []string
	for _, member := range plan.targetSubtree {
		if member.run.State.IsTerminal() {
			continue
		}
		if !member.hasSource {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: waiting cancellation target Run %q has no executor process",
				member.run.ID,
			)
		}
		expectedProcesses[member.source.ProcessID] = struct{}{}
		terminalRuns = append(terminalRuns, canceledWaitingRun(member.run, reason, finishedAt))
		canceledRunIDs = append(canceledRunIDs, member.run.ID)
	}
	if len(canceledProcessSet) != len(expectedProcesses) {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: prepared waiting cancellation removed %d processes, Run subtree requires %d",
			len(canceledProcessSet),
			len(expectedProcesses),
		)
	}
	for processID := range expectedProcesses {
		if _, canceled := canceledProcessSet[processID]; !canceled {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared waiting cancellation did not remove process %q",
				processID,
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

	continuations := make([]interrupts.Continuation, 0, len(plan.survivingTree))
	parentToolMoved := false
	for _, continuation := range plan.pending.Continuations {
		if _, canceled := canceledProcessSet[continuation.ProcessID]; canceled {
			continue
		}
		clone := continuation
		clone.DrainedTools = slices.Clone(continuation.DrainedTools)
		clone.CommittedTools = slices.Clone(continuation.CommittedTools)
		if continuation.RunID == plan.target.run.ParentRunID {
			var matches []interrupts.DrainedTool
			clone.DrainedTools = slices.DeleteFunc(clone.DrainedTools, func(tool interrupts.DrainedTool) bool {
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
			clone.CommittedTools = append(clone.CommittedTools, interrupts.CommittedTool{
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

	oldBindingByKey := make(map[string]int, len(plan.pending.Suspensions))
	for index, binding := range plan.pending.Suspensions {
		oldBindingByKey[suspensionIdentity(binding.ProcessID, binding.SuspensionID)] = index
	}
	survivingRunByProcess := make(map[string]string, len(continuations))
	for _, continuation := range continuations {
		survivingRunByProcess[continuation.ProcessID] = continuation.RunID
	}
	pendingSuspensions := prepared.PendingSuspensions()
	remainingInterrupts := make([]transcript.Interrupt, 0, len(pendingSuspensions))
	remainingBindings := make([]interrupts.SuspensionBinding, 0, len(pendingSuspensions))
	keptBindings := make(map[int]struct{}, len(pendingSuspensions))
	for _, boundary := range pendingSuspensions {
		if err := boundary.Interrupt.Validate(); err != nil {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared process %q suspension %q: %w",
				boundary.ProcessID,
				boundary.SuspensionID,
				err,
			)
		}
		index, exists := oldBindingByKey[suspensionIdentity(boundary.ProcessID, boundary.SuspensionID)]
		if !exists {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared process %q suspension %q was absent from the durable pending set",
				boundary.ProcessID,
				boundary.SuspensionID,
			)
		}
		if _, duplicate := keptBindings[index]; duplicate {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared process %q repeated suspension %q",
				boundary.ProcessID,
				boundary.SuspensionID,
			)
		}
		binding := plan.pending.Suspensions[index]
		interrupt := plan.pending.Interrupts[index]
		runID, survives := survivingRunByProcess[binding.ProcessID]
		if !survives || interrupt.RunID != runID {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared suspension %q belongs to removed process %q",
				binding.SuspensionID,
				binding.ProcessID,
			)
		}
		if interrupt.Kind != boundary.Interrupt.Kind {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared suspension %q changed interrupt kind from %s to %s",
				binding.SuspensionID,
				interrupt.Kind,
				boundary.Interrupt.Kind,
			)
		}
		keptBindings[index] = struct{}{}
		remainingInterrupts = append(remainingInterrupts, interrupt)
		remainingBindings = append(remainingBindings, binding)
	}
	for index, binding := range plan.pending.Suspensions {
		if _, kept := keptBindings[index]; kept {
			continue
		}
		if _, canceled := canceledProcessSet[binding.ProcessID]; !canceled {
			return waitingCancellationTransformation{}, fmt.Errorf(
				"runs: prepared cancellation dropped surviving process %q suspension %q",
				binding.ProcessID,
				binding.SuspensionID,
			)
		}
	}

	continuation := &treeContinuation{
		rootRunID:     plan.pending.RootRunID,
		sessionID:     plan.pending.SessionID,
		turnID:        plan.pending.TurnID,
		interrupts:    slices.Clone(remainingInterrupts),
		continuations: slices.Clone(continuations),
		profile:       plan.pending.ProtocolProfile,
	}
	if err := continuation.validate(); err != nil {
		return waitingCancellationTransformation{}, fmt.Errorf(
			"runs: waiting cancellation continuation: %w",
			err,
		)
	}

	var remaining *interrupts.Pending
	if len(remainingInterrupts) > 0 {
		reduced := plan.pending
		reduced.Interrupts = remainingInterrupts
		reduced.Suspensions = remainingBindings
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
		checkpoint:     prepared,
		root:           plan.root.run,
		targetRunID:    plan.target.run.ID,
		canceledRunIDs: canceledRunIDs,
	}, nil
}

func canceledWaitingRun(run transcript.Run, reason string, finishedAt time.Time) transcript.Run {
	outcome := execution.OutcomeCanceled
	run.State = execution.Canceled
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

func suspensionIdentity(processID, suspensionID string) string {
	return processID + "\x00" + suspensionID
}

func (transformation waitingCancellationTransformation) durableCommit(
	expected interrupts.Pending,
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
