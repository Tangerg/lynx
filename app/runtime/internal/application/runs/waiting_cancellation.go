package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	rundomain "github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// Validate verifies the Application projection and one-shot executor
// capability without interpreting the opaque checkpoint payload.
func (prepared PreparedWaitingSubtreeCancellation) Validate() error {
	if prepared.Change == nil {
		return errors.New("runs: prepared waiting subtree cancellation has no executor change")
	}
	if err := prepared.Checkpoint.Validate(); err != nil {
		return err
	}
	if len(prepared.CanceledMemberIDs) == 0 {
		return errors.New("runs: prepared waiting subtree cancellation has no canceled members")
	}
	canceledMembers := make(map[string]struct{}, len(prepared.CanceledMemberIDs))
	seenMembers := make(map[string]struct{}, len(prepared.CanceledMemberIDs)+len(prepared.PausedMemberIDs))
	for _, memberID := range prepared.CanceledMemberIDs {
		if strings.TrimSpace(memberID) == "" || memberID != strings.TrimSpace(memberID) {
			return errors.New("runs: prepared waiting subtree cancellation has an invalid canceled member ID")
		}
		if _, duplicate := seenMembers[memberID]; duplicate {
			return fmt.Errorf("runs: prepared waiting subtree cancellation repeats member %q", memberID)
		}
		canceledMembers[memberID] = struct{}{}
		seenMembers[memberID] = struct{}{}
	}
	for _, memberID := range prepared.PausedMemberIDs {
		if strings.TrimSpace(memberID) == "" || memberID != strings.TrimSpace(memberID) {
			return errors.New("runs: prepared waiting subtree cancellation has an invalid paused member ID")
		}
		if _, duplicate := seenMembers[memberID]; duplicate {
			return fmt.Errorf("runs: prepared waiting subtree cancellation repeats member %q", memberID)
		}
		seenMembers[memberID] = struct{}{}
	}
	requests := make(map[string]struct{}, len(prepared.PendingInterruptions))
	for index, interruption := range prepared.PendingInterruptions {
		if strings.TrimSpace(interruption.MemberID) == "" ||
			interruption.MemberID != strings.TrimSpace(interruption.MemberID) ||
			strings.TrimSpace(interruption.RequestID) == "" ||
			interruption.RequestID != strings.TrimSpace(interruption.RequestID) {
			return fmt.Errorf("runs: prepared waiting subtree interruption[%d] has invalid identity", index)
		}
		if _, canceled := canceledMembers[interruption.MemberID]; canceled {
			return fmt.Errorf(
				"runs: prepared waiting subtree interruption[%d] belongs to canceled member %q",
				index,
				interruption.MemberID,
			)
		}
		if err := interruption.Interrupt.Validate(); err != nil {
			return fmt.Errorf("runs: prepared waiting subtree interruption[%d]: %w", index, err)
		}
		identity := inputRequestIdentity(interruption.MemberID, interruption.RequestID)
		if _, duplicate := requests[identity]; duplicate {
			return fmt.Errorf("runs: prepared waiting subtree cancellation repeats interruption %q", identity)
		}
		requests[identity] = struct{}{}
	}
	return nil
}

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

// waitingCancellationBuilder owns the pure Application transformation from a
// command-bound cancellation plan and an executor-prepared change to the one
// durable write-set. It does not execute the one-shot change or interpret its
// opaque checkpoint.
type waitingCancellationBuilder struct {
	plan       cancellationPlan
	reason     string
	finishedAt time.Time
	prepared   PreparedWaitingSubtreeCancellation
}

func prepareWaitingCancellationTransformation(
	plan cancellationPlan,
	reason string,
	finishedAt time.Time,
	prepared PreparedWaitingSubtreeCancellation,
) (waitingCancellationTransformation, error) {
	builder := waitingCancellationBuilder{
		plan: plan, reason: reason, finishedAt: finishedAt, prepared: prepared,
	}
	return builder.build()
}

func (builder waitingCancellationBuilder) build() (waitingCancellationTransformation, error) {
	if err := builder.validate(); err != nil {
		return waitingCancellationTransformation{}, err
	}
	canceledMembers := make(map[string]struct{}, len(builder.prepared.CanceledMemberIDs))
	for _, memberID := range builder.prepared.CanceledMemberIDs {
		canceledMembers[memberID] = struct{}{}
	}

	terminalRuns, canceledRunIDs, err := builder.terminalProjection(canceledMembers)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	terminalItems, parentItem, continuations, err := builder.settleWaitingItems(canceledMembers)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	interrupts, bindings, err := builder.remainingInterruptions(canceledMembers, continuations)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	continuation, err := builder.treeContinuation(interrupts, continuations)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	remaining, err := builder.remainingPending(interrupts, bindings, continuations)
	if err != nil {
		return waitingCancellationTransformation{}, err
	}
	return waitingCancellationTransformation{
		terminalRuns:   terminalRuns,
		terminalItems:  terminalItems,
		parentItem:     parentItem,
		remaining:      remaining,
		continuation:   continuation,
		checkpoint:     builder.prepared.Checkpoint.Clone(),
		root:           builder.plan.root.run,
		targetRunID:    builder.plan.target.run.ID,
		canceledRunIDs: canceledRunIDs,
	}, nil
}

func (builder waitingCancellationBuilder) validate() error {
	if err := builder.prepared.Validate(); err != nil {
		return err
	}
	switch {
	case builder.plan.treeState != rundomain.Waiting:
		return fmt.Errorf(
			"runs: waiting cancellation plan is %s",
			builder.plan.treeState,
		)
	case !builder.plan.target.run.Lineage().IsChild():
		return errors.New("runs: waiting cancellation target is not a child Run")
	case !builder.plan.hasPending:
		return errors.New("runs: waiting cancellation plan has no pending set")
	case !builder.plan.hasSpawningItem:
		return errors.New("runs: waiting cancellation plan has no spawning Item")
	case builder.finishedAt.IsZero():
		return errors.New("runs: waiting cancellation finish time is required")
	}
	rootContinuation, ok := builder.plan.pending.RootContinuation()
	if !ok {
		return errors.New("runs: waiting cancellation Pending has no root continuation")
	}
	if err := builder.prepared.Checkpoint.ValidateOwnership(
		rootContinuation.MemberID,
		builder.plan.pending.SessionID,
	); err != nil {
		return fmt.Errorf("runs: invalid prepared waiting subtree checkpoint ownership: %w", err)
	}
	if builder.prepared.Checkpoint.Scope.GoalLeaseID != builder.plan.pending.GoalLeaseID {
		return fmt.Errorf(
			"runs: prepared waiting subtree checkpoint goal lease %q does not match Pending %q: %w",
			builder.prepared.Checkpoint.Scope.GoalLeaseID,
			builder.plan.pending.GoalLeaseID,
			ErrInvalidExecutorCheckpoint,
		)
	}
	if builder.prepared.Checkpoint.ModelSelection != rootContinuation.ModelSelection {
		return fmt.Errorf(
			"runs: prepared waiting subtree checkpoint model %q/%q does not match root continuation %q/%q: %w",
			builder.prepared.Checkpoint.ModelSelection.Provider(),
			builder.prepared.Checkpoint.ModelSelection.Model(),
			rootContinuation.ModelSelection.Provider(),
			rootContinuation.ModelSelection.Model(),
			ErrInvalidExecutorCheckpoint,
		)
	}
	if builder.prepared.Checkpoint.Limits != rootContinuation.Limits {
		return fmt.Errorf(
			"runs: prepared waiting subtree checkpoint limits %+v do not match root continuation %+v: %w",
			builder.prepared.Checkpoint.Limits,
			rootContinuation.Limits,
			ErrInvalidExecutorCheckpoint,
		)
	}
	return nil
}

func (builder waitingCancellationBuilder) terminalProjection(
	canceledMembers map[string]struct{},
) ([]transcript.Run, []string, error) {
	expectedProcesses := make(map[string]struct{})
	var terminalRuns []transcript.Run
	var canceledRunIDs []string
	for _, member := range builder.plan.targetSubtree {
		if member.run.State.IsTerminal() {
			continue
		}
		if !member.hasMember {
			return nil, nil, fmt.Errorf(
				"runs: waiting cancellation target Run %q has no executor member",
				member.run.ID,
			)
		}
		expectedProcesses[member.memberID] = struct{}{}
		terminalRuns = append(terminalRuns, canceledWaitingRun(member.run, builder.reason, builder.finishedAt))
		canceledRunIDs = append(canceledRunIDs, member.run.ID)
	}
	if len(canceledMembers) != len(expectedProcesses) {
		return nil, nil, fmt.Errorf(
			"runs: prepared waiting cancellation removed %d members, Run subtree requires %d",
			len(canceledMembers),
			len(expectedProcesses),
		)
	}
	for memberID := range expectedProcesses {
		if _, canceled := canceledMembers[memberID]; !canceled {
			return nil, nil, fmt.Errorf(
				"runs: prepared waiting cancellation did not remove member %q",
				memberID,
			)
		}
	}
	return terminalRuns, canceledRunIDs, nil
}

func (builder waitingCancellationBuilder) settleWaitingItems(
	canceledMembers map[string]struct{},
) ([]ItemReplacement, ItemReplacement, []Continuation, error) {
	problem := transcript.Problem{
		Kind:   transcript.ChildRunCanceledProblem,
		Scope:  transcript.ToolProblem,
		Detail: builder.reason,
	}
	parentItem := builder.plan.spawningItem
	replacement := parentItem
	replacement.Status = transcript.ItemIncomplete
	replacement.Error = &problem
	terminalItems := make([]ItemReplacement, 0, len(builder.plan.targetInterruptItems))
	for _, item := range builder.plan.targetInterruptItems {
		settled := item
		settled.Status = transcript.ItemIncomplete
		if settled.Kind == transcript.ToolCall {
			settled.FinishedAt = builder.finishedAt.UTC()
			settled.Error = &transcript.Problem{
				Kind:   transcript.ToolFailedProblem,
				Scope:  transcript.ToolProblem,
				Detail: builder.reason,
			}
		}
		terminalItems = append(terminalItems, ItemReplacement{
			Expected:    item,
			Replacement: settled,
		})
	}

	continuations := make([]Continuation, 0, len(builder.plan.survivingTree))
	parentToolMoved := false
	for _, continuation := range builder.plan.pending.Continuations {
		if _, canceled := canceledMembers[continuation.MemberID]; canceled {
			continue
		}
		clone := continuation
		clone.DrainedTools = slices.Clone(continuation.DrainedTools)
		clone.CommittedTools = slices.Clone(continuation.CommittedTools)
		if continuation.RunID == builder.plan.target.run.ParentRunID {
			var matches []DrainedTool
			clone.DrainedTools = slices.DeleteFunc(clone.DrainedTools, func(tool DrainedTool) bool {
				if tool.ItemID != parentItem.ID {
					return false
				}
				matches = append(matches, tool)
				return true
			})
			if len(matches) != 1 {
				return nil, ItemReplacement{}, nil, fmt.Errorf(
					"runs: parent Run %q continuation has %d drained tools for spawning Item %q",
					continuation.RunID,
					len(matches),
					parentItem.ID,
				)
			}
			tool := matches[0]
			if tool.Name != parentItem.Tool.Name ||
				tool.Arguments != parentItem.Tool.Arguments.Canonical() {
				return nil, ItemReplacement{}, nil, fmt.Errorf(
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
		return nil, ItemReplacement{}, nil, fmt.Errorf(
			"runs: waiting cancellation did not settle spawning Item %q",
			parentItem.ID,
		)
	}
	return terminalItems, ItemReplacement{Expected: parentItem, Replacement: replacement}, continuations, nil
}

func (builder waitingCancellationBuilder) remainingInterruptions(
	canceledMembers map[string]struct{},
	continuations []Continuation,
) ([]transcript.Interrupt, []InterruptBinding, error) {
	oldBindingByKey := make(map[string]int, len(builder.plan.pending.Bindings))
	for index, binding := range builder.plan.pending.Bindings {
		oldBindingByKey[inputRequestIdentity(binding.MemberID, binding.RequestID)] = index
	}
	survivingRunByMemberID := make(map[string]string, len(continuations))
	for _, continuation := range continuations {
		survivingRunByMemberID[continuation.MemberID] = continuation.RunID
	}
	pendingInterruptions := builder.prepared.PendingInterruptions
	remainingInterrupts := make([]transcript.Interrupt, 0, len(pendingInterruptions))
	remainingBindings := make([]InterruptBinding, 0, len(pendingInterruptions))
	keptBindings := make(map[int]struct{}, len(pendingInterruptions))
	for _, boundary := range pendingInterruptions {
		if err := boundary.Interrupt.Validate(); err != nil {
			return nil, nil, fmt.Errorf(
				"runs: prepared member %q input request %q: %w",
				boundary.MemberID,
				boundary.RequestID,
				err,
			)
		}
		index, exists := oldBindingByKey[inputRequestIdentity(boundary.MemberID, boundary.RequestID)]
		if !exists {
			return nil, nil, fmt.Errorf(
				"runs: prepared member %q input request %q was absent from the durable pending set",
				boundary.MemberID,
				boundary.RequestID,
			)
		}
		if _, duplicate := keptBindings[index]; duplicate {
			return nil, nil, fmt.Errorf(
				"runs: prepared member %q repeated input request %q",
				boundary.MemberID,
				boundary.RequestID,
			)
		}
		binding := builder.plan.pending.Bindings[index]
		interrupt := builder.plan.pending.Interrupts[index]
		runID, survives := survivingRunByMemberID[binding.MemberID]
		if !survives || interrupt.RunID != runID {
			return nil, nil, fmt.Errorf(
				"runs: prepared input request %q belongs to removed member %q",
				binding.RequestID,
				binding.MemberID,
			)
		}
		if interrupt.Kind != boundary.Interrupt.Kind {
			return nil, nil, fmt.Errorf(
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
	for index, binding := range builder.plan.pending.Bindings {
		if _, kept := keptBindings[index]; kept {
			continue
		}
		if _, canceled := canceledMembers[binding.MemberID]; !canceled {
			return nil, nil, fmt.Errorf(
				"runs: prepared cancellation dropped surviving member %q input request %q",
				binding.MemberID,
				binding.RequestID,
			)
		}
	}
	return remainingInterrupts, remainingBindings, nil
}

func (builder waitingCancellationBuilder) treeContinuation(
	interrupts []transcript.Interrupt,
	continuations []Continuation,
) (*treeContinuation, error) {
	continuation := &treeContinuation{
		rootRunID:     builder.plan.pending.RootRunID,
		sessionID:     builder.plan.pending.SessionID,
		executorID:    builder.plan.pending.ExecutorID,
		goalLeaseID:   builder.plan.pending.GoalLeaseID,
		interrupts:    slices.Clone(interrupts),
		continuations: slices.Clone(continuations),
		capabilities:  builder.plan.pending.Capabilities,
	}
	if err := continuation.validate(); err != nil {
		return nil, fmt.Errorf(
			"runs: waiting cancellation continuation: %w",
			err,
		)
	}
	return continuation, nil
}

func (builder waitingCancellationBuilder) remainingPending(
	interrupts []transcript.Interrupt,
	bindings []InterruptBinding,
	continuations []Continuation,
) (*Pending, error) {
	if len(interrupts) == 0 {
		return nil, nil
	}
	reduced := builder.plan.pending
	reduced.Interrupts = interrupts
	reduced.Bindings = bindings
	reduced.Continuations = continuations
	if err := reduced.Validate(); err != nil {
		return nil, fmt.Errorf(
			"runs: reduced waiting cancellation pending set: %w",
			err,
		)
	}
	return &reduced, nil
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
