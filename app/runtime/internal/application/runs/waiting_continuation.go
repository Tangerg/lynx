package runs

import (
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// WaitingSubtreeCancellationRequest carries the complete durable waiting tree
// and the addressed child cancellation into the execution port. Its implementation
// may claim the live tree or restore this exact checkpoint, but cannot reread
// Application persistence.
type WaitingSubtreeCancellationRequest struct {
	Continuation   WaitingContinuation
	TargetMemberID string
	Reason         string
}

// Validate verifies the Application-owned waiting subtree command without
// interpreting the executor checkpoint payload.
func (w WaitingSubtreeCancellationRequest) Validate() error {
	if err := w.Continuation.Validate(); err != nil {
		return fmt.Errorf("runs: waiting subtree continuation: %w", err)
	}
	if strings.TrimSpace(w.TargetMemberID) == "" ||
		w.TargetMemberID != strings.TrimSpace(w.TargetMemberID) {
		return errors.New("runs: waiting subtree target member ID is required without surrounding whitespace")
	}
	if strings.TrimSpace(w.Reason) == "" || w.Reason != strings.TrimSpace(w.Reason) {
		return errors.New("runs: waiting subtree reason is required without surrounding whitespace")
	}
	targetFound := false
	for _, member := range w.Continuation.Members {
		if member.MemberID != w.TargetMemberID {
			continue
		}
		if member.ParentRunID == "" {
			return errors.New("runs: waiting subtree target is the root member")
		}
		targetFound = true
		break
	}
	if !targetFound {
		return errors.New("runs: waiting subtree target is absent from the continuation")
	}
	return nil
}

func waitingContinuationFromPending(
	pending Pending,
	checkpoint ExecutorCheckpoint,
) (WaitingContinuation, error) {
	if err := pending.Validate(); err != nil {
		return WaitingContinuation{}, err
	}
	members := make([]WaitingMember, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		members[index] = WaitingMember{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			ParentRunID:     continuation.Lineage.ParentRunID,
			SpawnedByItemID: continuation.Lineage.SpawnedByItemID,
			ModelSelection:  continuation.ModelSelection, Metrics: continuation.Metrics,
		}
	}
	result := WaitingContinuation{
		SessionID: pending.SessionID, ExecutorID: pending.ExecutorID,
		RootRunID: pending.RootRunID, Members: members, Checkpoint: checkpoint.Clone(),
		Capabilities:             pending.Capabilities,
		ChildRunAdmissionEnabled: pending.Capabilities.ChildRuns,
	}
	if err := result.Validate(); err != nil {
		return WaitingContinuation{}, err
	}
	return result, nil
}

// Validate verifies one surviving product member without interpreting executor
// topology or checkpoint payload.
func (w WaitingMember) Validate() error {
	if err := validateRequiredIdentity("Run ID", w.RunID); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if err := validateRequiredIdentity("member ID", w.MemberID); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if w.ParentRunID != strings.TrimSpace(w.ParentRunID) || w.ParentRunID == w.RunID ||
		w.SpawnedByItemID != strings.TrimSpace(w.SpawnedByItemID) {
		return errors.New("runs: waiting member has invalid parent Run identity")
	}
	if (w.ParentRunID == "") != (w.SpawnedByItemID == "") {
		return errors.New("runs: waiting member child lineage is incomplete")
	}
	if err := w.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("runs: waiting member model selection: %w", err)
	}
	if err := w.Metrics.Validate(); err != nil {
		return fmt.Errorf("runs: waiting member metrics: %w", err)
	}
	return nil
}

// Validate verifies the complete Application side of one executor continuation.
// The opaque checkpoint payload remains the executor implementation's responsibility.
func (w WaitingContinuation) Validate() error {
	if err := validateWaitingContinuationEnvelope(w); err != nil {
		return err
	}
	topology, err := buildWaitingContinuationTopology(w)
	if err != nil {
		return err
	}
	if err := validateWaitingContinuationOrder(w.Members, topology.tree.Postorder()); err != nil {
		return err
	}
	if len(w.Members) > 1 && !w.Capabilities.ChildRuns {
		return errors.New("runs: waiting continuation has child members without child-Run capability")
	}
	return w.Checkpoint.ValidateOwnership(topology.rootMemberID, w.SessionID)
}

func validateWaitingContinuationEnvelope(continuation WaitingContinuation) error {
	if err := validateRequiredIdentity("Session ID", continuation.SessionID); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if err := validateRequiredIdentity("executor ID", continuation.ExecutorID); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if err := validateRequiredIdentity("root Run ID", continuation.RootRunID); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if len(continuation.Members) == 0 {
		return errors.New("runs: waiting continuation has no surviving members")
	}
	if err := continuation.Capabilities.Validate(); err != nil {
		return fmt.Errorf("runs: waiting continuation capabilities: %w", err)
	}
	if continuation.ChildRunAdmissionEnabled != continuation.Capabilities.ChildRuns {
		return errors.New("runs: waiting continuation child admission differs from frozen capabilities")
	}
	return nil
}

type waitingContinuationTopology struct {
	rootMemberID string
	tree         run.Tree
}

func buildWaitingContinuationTopology(
	continuation WaitingContinuation,
) (waitingContinuationTopology, error) {
	seenRunIDs := make(map[string]struct{}, len(continuation.Members))
	seenMemberIDs := make(map[string]struct{}, len(continuation.Members))
	treeMembers := make([]run.TreeMember, 0, len(continuation.Members))
	rootMemberID := ""
	for index, member := range continuation.Members {
		if err := member.Validate(); err != nil {
			return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation member[%d]: %w", index, err)
		}
		if _, duplicate := seenRunIDs[member.RunID]; duplicate {
			return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation repeats Run %q", member.RunID)
		}
		if _, duplicate := seenMemberIDs[member.MemberID]; duplicate {
			return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation repeats member %q", member.MemberID)
		}
		seenRunIDs[member.RunID] = struct{}{}
		seenMemberIDs[member.MemberID] = struct{}{}
		lineage := run.Lineage{}
		if member.RunID != continuation.RootRunID {
			if member.ParentRunID == "" {
				return waitingContinuationTopology{}, fmt.Errorf("runs: waiting child Run %q has no parent", member.RunID)
			}
			lineage = run.Lineage{
				SpawnedByItemID: member.SpawnedByItemID,
				ParentRunID:     member.ParentRunID, RootRunID: continuation.RootRunID,
			}
		} else {
			if member.ParentRunID != "" || rootMemberID != "" {
				return waitingContinuationTopology{}, errors.New("runs: waiting continuation has an invalid root member")
			}
			rootMemberID = member.MemberID
		}
		treeMembers = append(treeMembers, run.TreeMember{RunID: member.RunID, Lineage: lineage})
	}
	if rootMemberID == "" {
		return waitingContinuationTopology{}, errors.New("runs: waiting continuation has no root member")
	}
	tree, err := run.NewTree(continuation.RootRunID, treeMembers)
	if err != nil {
		return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation product tree: %w", err)
	}
	return waitingContinuationTopology{rootMemberID: rootMemberID, tree: tree}, nil
}

func validateWaitingContinuationOrder(members []WaitingMember, canonicalRunIDs []string) error {
	for index, member := range members {
		if member.RunID != canonicalRunIDs[index] {
			return fmt.Errorf(
				"runs: waiting continuation member[%d] is Run %q, canonical postorder requires %q",
				index, member.RunID, canonicalRunIDs[index],
			)
		}
	}
	return nil
}
