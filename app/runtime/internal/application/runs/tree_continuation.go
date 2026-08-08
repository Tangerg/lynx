package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// treeContinuation is the application-private execution hand-off shared by
// human-driven Resume and a host-settled continuation. It deliberately does not
// mean "an interrupt is open": Pending owns that external-input fact, while a
// tree whose final interrupt was canceled still has enough continuation state
// to open fresh Segments without inventing a fake human answer.
type treeContinuation struct {
	rootRunID     string
	sessionID     string
	executorID    string
	goalLeaseID   string
	interrupts    []transcript.Interrupt
	continuations []Continuation
	capabilities  run.RunCapabilities
}

func treeContinuationFromPending(pending Pending) (*treeContinuation, error) {
	if err := pending.Validate(); err != nil {
		return nil, err
	}
	continuation := &treeContinuation{
		rootRunID:     pending.RootRunID,
		sessionID:     pending.SessionID,
		executorID:    pending.ExecutorID,
		goalLeaseID:   pending.GoalLeaseID,
		interrupts:    slices.Clone(pending.Interrupts),
		continuations: slices.Clone(pending.Continuations),
		capabilities:  pending.Capabilities,
	}
	if err := continuation.validate(); err != nil {
		return nil, err
	}
	return continuation, nil
}

func (continuation *treeContinuation) validate() error {
	if continuation == nil {
		return errors.New("runs: tree continuation is required")
	}
	switch {
	case strings.TrimSpace(continuation.rootRunID) == "":
		return errors.New("runs: tree continuation root Run id is required")
	case strings.TrimSpace(continuation.sessionID) == "":
		return errors.New("runs: tree continuation Session id is required")
	case strings.TrimSpace(continuation.executorID) == "":
		return errors.New("runs: tree continuation executor ID is required")
	case continuation.goalLeaseID != strings.TrimSpace(continuation.goalLeaseID):
		return errors.New("runs: tree continuation goal lease id has surrounding whitespace")
	case len(continuation.continuations) == 0:
		return errors.New("runs: tree continuation has no Runs")
	}

	runIDs := make(map[string]struct{}, len(continuation.continuations))
	memberOwners := make(map[string]string, len(continuation.continuations))
	members := make([]run.RunTreeMember, 0, len(continuation.continuations))
	for index, member := range continuation.continuations {
		if err := member.Validate(); err != nil {
			return fmt.Errorf("runs: tree continuation Run[%d]: %w", index, err)
		}
		if _, duplicate := runIDs[member.RunID]; duplicate {
			return fmt.Errorf("runs: tree continuation repeats Run %q", member.RunID)
		}
		if owner, duplicate := memberOwners[member.MemberID]; duplicate {
			return fmt.Errorf(
				"runs: tree continuation member %q belongs to Runs %q and %q",
				member.MemberID,
				owner,
				member.RunID,
			)
		}
		runIDs[member.RunID] = struct{}{}
		memberOwners[member.MemberID] = member.RunID
		members = append(members, run.RunTreeMember{
			RunID:   member.RunID,
			Lineage: member.Lineage,
		})
	}
	tree, err := run.NewRunTree(continuation.rootRunID, members)
	if err != nil {
		return fmt.Errorf("runs: tree continuation topology: %w", err)
	}
	canonical := tree.Postorder()
	for index, member := range continuation.continuations {
		if member.RunID != canonical[index] {
			return fmt.Errorf(
				"runs: tree continuation Run[%d] is %q, canonical postorder requires %q",
				index,
				member.RunID,
				canonical[index],
			)
		}
	}
	for index, interrupt := range continuation.interrupts {
		if interrupt.ItemID == "" || interrupt.RunID == "" {
			return fmt.Errorf("runs: tree continuation interrupt[%d] has incomplete identity", index)
		}
		if _, exists := runIDs[interrupt.RunID]; !exists {
			return fmt.Errorf(
				"runs: tree continuation interrupt[%d] names removed Run %q",
				index,
				interrupt.RunID,
			)
		}
	}
	return nil
}

func (continuation *treeContinuation) root() (Continuation, bool) {
	if continuation == nil {
		return Continuation{}, false
	}
	for _, member := range continuation.continuations {
		if member.RunID == continuation.rootRunID {
			return member, true
		}
	}
	return Continuation{}, false
}

func (continuation *treeContinuation) forRun(runID string) (Continuation, bool) {
	if continuation == nil {
		return Continuation{}, false
	}
	for _, member := range continuation.continuations {
		if member.RunID == runID {
			return member, true
		}
	}
	return Continuation{}, false
}
