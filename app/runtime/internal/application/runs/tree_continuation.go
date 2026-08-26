package runs

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// treeContinuation is the application-private execution hand-off shared by
// human-driven Resume and a host-settled continuation. It deliberately does not
// mean "an interrupt is open": Pending owns that external-input fact, while a
// tree whose final interrupt was canceled still has enough continuation state
// to open fresh Segments without inventing a fake human answer.
type treeContinuation struct {
	rootRunID           string
	sessionID           string
	executorID          string
	goalIncarnationID   string
	interrupts          []transcript.Interrupt
	approvalResolutions map[string]ToolApprovalResolution
	continuations       []Continuation
	capabilities        run.Capabilities
}

func (t *treeContinuation) bindToolApprovalResolutions(
	resolutions []ToolApprovalResolution,
) error {
	if t == nil {
		return errors.New("runs: tree continuation is required")
	}
	if len(resolutions) == 0 {
		return nil
	}
	interruptItems := make(map[string]transcript.Interrupt, len(t.interrupts))
	for _, pending := range t.interrupts {
		interruptItems[pending.ItemID] = pending
	}
	resolved := make(map[string]ToolApprovalResolution, len(resolutions))
	for _, resolution := range resolutions {
		if err := resolution.Validate(); err != nil {
			return err
		}
		if resolution.Identity.SessionID != t.sessionID {
			return fmt.Errorf("runs: Tool approval item %q belongs to another Session", resolution.Identity.ItemID)
		}
		pending, exists := interruptItems[resolution.Identity.ItemID]
		if !exists {
			return fmt.Errorf("runs: Tool approval item %q is not in the continuation", resolution.Identity.ItemID)
		}
		if pending.Kind != interrupt.Approval || pending.Approval == nil ||
			pending.RunID != resolution.Identity.RunID ||
			!pending.ItemOccurredAt.Equal(resolution.Identity.OccurredAt) ||
			!reflect.DeepEqual(pending.Approval.Tool, resolution.Invocation) {
			return fmt.Errorf("runs: Tool approval item %q differs from the continuation", resolution.Identity.ItemID)
		}
		if _, duplicate := resolved[resolution.Identity.ItemID]; duplicate {
			return fmt.Errorf("runs: Tool approval item %q is resolved twice", resolution.Identity.ItemID)
		}
		resolved[resolution.Identity.ItemID] = resolution
	}
	t.approvalResolutions = resolved
	return nil
}

func treeContinuationFromPending(pending Pending) (*treeContinuation, error) {
	if err := pending.Validate(); err != nil {
		return nil, err
	}
	continuation := &treeContinuation{
		rootRunID:         pending.RootRunID,
		sessionID:         pending.SessionID,
		executorID:        pending.ExecutorID,
		goalIncarnationID: pending.GoalIncarnationID,
		interrupts:        slices.Clone(pending.Interrupts),
		continuations:     slices.Clone(pending.Continuations),
		capabilities:      pending.Capabilities,
	}
	if err := continuation.validate(); err != nil {
		return nil, err
	}
	return continuation, nil
}

func (t *treeContinuation) validate() error {
	if t == nil {
		return errors.New("runs: tree continuation is required")
	}
	switch {
	case strings.TrimSpace(t.rootRunID) == "":
		return errors.New("runs: tree continuation root Run id is required")
	case strings.TrimSpace(t.sessionID) == "":
		return errors.New("runs: tree continuation Session id is required")
	case strings.TrimSpace(t.executorID) == "":
		return errors.New("runs: tree continuation executor ID is required")
	case t.goalIncarnationID != strings.TrimSpace(t.goalIncarnationID):
		return errors.New("runs: tree continuation goal incarnation id has surrounding whitespace")
	case len(t.continuations) == 0:
		return errors.New("runs: tree continuation has no Runs")
	}

	runIDs := make(map[string]struct{}, len(t.continuations))
	memberOwners := make(map[string]string, len(t.continuations))
	members := make([]run.TreeMember, 0, len(t.continuations))
	for index, member := range t.continuations {
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
		members = append(members, run.TreeMember{
			RunID:   member.RunID,
			Lineage: member.Lineage,
		})
	}
	tree, err := run.NewTree(t.rootRunID, members)
	if err != nil {
		return fmt.Errorf("runs: tree continuation topology: %w", err)
	}
	canonical := tree.Postorder()
	for index, member := range t.continuations {
		if member.RunID != canonical[index] {
			return fmt.Errorf(
				"runs: tree continuation Run[%d] is %q, canonical postorder requires %q",
				index,
				member.RunID,
				canonical[index],
			)
		}
	}
	for index, interrupt := range t.interrupts {
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

func (t *treeContinuation) root() (Continuation, bool) {
	if t == nil {
		return Continuation{}, false
	}
	for _, member := range t.continuations {
		if member.RunID == t.rootRunID {
			return member, true
		}
	}
	return Continuation{}, false
}

func (t *treeContinuation) forRun(runID string) (Continuation, bool) {
	if t == nil {
		return Continuation{}, false
	}
	for _, member := range t.continuations {
		if member.RunID == runID {
			return member, true
		}
	}
	return Continuation{}, false
}
