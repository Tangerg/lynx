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
func (request WaitingSubtreeCancellationRequest) Validate() error {
	if err := request.Continuation.Validate(); err != nil {
		return fmt.Errorf("runs: waiting subtree continuation: %w", err)
	}
	if strings.TrimSpace(request.TargetMemberID) == "" ||
		request.TargetMemberID != strings.TrimSpace(request.TargetMemberID) {
		return errors.New("runs: waiting subtree target member ID is required without surrounding whitespace")
	}
	if strings.TrimSpace(request.Reason) == "" || request.Reason != strings.TrimSpace(request.Reason) {
		return errors.New("runs: waiting subtree reason is required without surrounding whitespace")
	}
	targetFound := false
	for _, member := range request.Continuation.Members {
		if member.MemberID != request.TargetMemberID {
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
func (member WaitingMember) Validate() error {
	for name, value := range map[string]string{
		"Run ID": member.RunID, "member ID": member.MemberID,
	} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("runs: waiting member %s is required without surrounding whitespace", name)
		}
	}
	if member.ParentRunID != strings.TrimSpace(member.ParentRunID) || member.ParentRunID == member.RunID ||
		member.SpawnedByItemID != strings.TrimSpace(member.SpawnedByItemID) {
		return errors.New("runs: waiting member has invalid parent Run identity")
	}
	if (member.ParentRunID == "") != (member.SpawnedByItemID == "") {
		return errors.New("runs: waiting member child lineage is incomplete")
	}
	if err := member.ModelSelection.Validate(); err != nil {
		return fmt.Errorf("runs: waiting member model selection: %w", err)
	}
	if err := member.Metrics.Validate(); err != nil {
		return fmt.Errorf("runs: waiting member metrics: %w", err)
	}
	return nil
}

// Validate verifies the complete Application side of one executor continuation.
// The opaque checkpoint payload remains the executor implementation's responsibility.
func (continuation WaitingContinuation) Validate() error {
	for name, value := range map[string]string{
		"Session ID":  continuation.SessionID,
		"executor ID": continuation.ExecutorID,
		"root Run ID": continuation.RootRunID,
	} {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("runs: waiting continuation %s is required without surrounding whitespace", name)
		}
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
	runIDs := make(map[string]struct{}, len(continuation.Members))
	memberIDs := make(map[string]struct{}, len(continuation.Members))
	treeMembers := make([]run.TreeMember, 0, len(continuation.Members))
	rootMemberID := ""
	for index, member := range continuation.Members {
		if err := member.Validate(); err != nil {
			return fmt.Errorf("runs: waiting continuation member[%d]: %w", index, err)
		}
		if _, duplicate := runIDs[member.RunID]; duplicate {
			return fmt.Errorf("runs: waiting continuation repeats Run %q", member.RunID)
		}
		if _, duplicate := memberIDs[member.MemberID]; duplicate {
			return fmt.Errorf("runs: waiting continuation repeats member %q", member.MemberID)
		}
		runIDs[member.RunID] = struct{}{}
		memberIDs[member.MemberID] = struct{}{}
		lineage := run.Lineage{}
		if member.RunID != continuation.RootRunID {
			if member.ParentRunID == "" {
				return fmt.Errorf("runs: waiting child Run %q has no parent", member.RunID)
			}
			lineage = run.Lineage{
				SpawnedByItemID: member.SpawnedByItemID,
				ParentRunID:     member.ParentRunID, RootRunID: continuation.RootRunID,
			}
		} else {
			if member.ParentRunID != "" || rootMemberID != "" {
				return errors.New("runs: waiting continuation has an invalid root member")
			}
			rootMemberID = member.MemberID
		}
		treeMembers = append(treeMembers, run.TreeMember{RunID: member.RunID, Lineage: lineage})
	}
	if rootMemberID == "" {
		return errors.New("runs: waiting continuation has no root member")
	}
	tree, err := run.NewTree(continuation.RootRunID, treeMembers)
	if err != nil {
		return fmt.Errorf("runs: waiting continuation product tree: %w", err)
	}
	postorder := tree.Postorder()
	for index, member := range continuation.Members {
		if member.RunID != postorder[index] {
			return fmt.Errorf(
				"runs: waiting continuation member[%d] is Run %q, canonical postorder requires %q",
				index, member.RunID, postorder[index],
			)
		}
	}
	if len(continuation.Members) > 1 && !continuation.Capabilities.ChildRuns {
		return errors.New("runs: waiting continuation has child members without child-Run capability")
	}
	if err := continuation.Checkpoint.ValidateOwnership(rootMemberID, continuation.SessionID); err != nil {
		return err
	}
	return nil
}
