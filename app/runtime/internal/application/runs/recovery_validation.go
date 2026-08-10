package runs

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// validateRecoveryParkedTree checks the complete durable hand-off barrier before
// boot keeps a Run tree resumable. Pending is root-owned, so its continuations
// must cover every non-terminal member exactly once.
//
// An impossible partial application write is corruption and fails startup. A
// missing or executor-incompatible checkpoint is an external-resource loss and
// returns resumable=false so recovery can mark the whole tree run_lost.
func validateRecoveryParkedTree(
	ctx context.Context,
	tree recoveryRunTree,
	pending Pending,
	sess session.Session,
	items []transcript.Item,
	checkpoints CheckpointResumability,
) (bool, error) {
	if err := pending.Validate(); err != nil {
		return false, fmt.Errorf("runs: validate recovery Run tree %q Pending: %w", tree.root.ID(), err)
	}
	if pending.RootRunID != tree.root.ID() || pending.SessionID != tree.root.SessionID() {
		return false, fmt.Errorf(
			"runs: validate recovery Run tree %q: Pending identity is %q/%q, want %q/%q",
			tree.root.ID(),
			pending.SessionID,
			pending.RootRunID,
			tree.root.SessionID(),
			tree.root.ID(),
		)
	}
	if pending.GoalLeaseID != tree.root.GoalLeaseID() {
		return false, fmt.Errorf(
			"runs: validate recovery Run tree %q: Pending goal lease %q differs from root Run lease %q",
			tree.root.ID(),
			pending.GoalLeaseID,
			tree.root.GoalLeaseID(),
		)
	}
	if !pending.Capabilities.Equal(tree.root.Capabilities()) {
		return false, fmt.Errorf(
			"runs: validate recovery Run tree %q: Pending run capabilities differ from root Run admission",
			tree.root.ID(),
		)
	}
	if len(pending.Continuations) != len(tree.runsByID) {
		return false, fmt.Errorf(
			"runs: validate recovery Run tree %q: Pending has %d continuations, want %d active Runs",
			tree.root.ID(),
			len(pending.Continuations),
			len(tree.runsByID),
		)
	}
	for _, continuation := range pending.Continuations {
		active, found := tree.runsByID[continuation.RunID]
		if !found {
			return false, fmt.Errorf(
				"runs: validate recovery Run tree %q: continuation names non-active Run %q",
				tree.root.ID(),
				continuation.RunID,
			)
		}
		if err := validateRecoveryContinuation(active, tree.root, continuation); err != nil {
			return false, err
		}
	}

	itemsByID := indexRecoveryItems(items)
	interruptItems, err := validateRecoveryInterruptItems(tree, pending.Interrupts, itemsByID)
	if err != nil {
		return false, err
	}
	if err := validateRecoveryRunningItems(tree, items, interruptItems); err != nil {
		return false, err
	}
	claimedItems := make(map[string]string, len(interruptItems))
	for itemID := range interruptItems {
		claimedItems[itemID] = "interrupt"
	}
	for _, continuation := range pending.Continuations {
		if err := validateRecoveryContinuationTools(
			tree.root.ID(),
			continuation,
			itemsByID,
			claimedItems,
		); err != nil {
			return false, err
		}
	}

	rootContinuation, _ := pending.RootContinuation()
	// Isolated workspaces are process-local scratch copies and are deliberately
	// never snapshotted. A host restart therefore destroys the world this tree
	// was parked in even when its executor payload remains decodable.
	if sess.Isolated {
		return false, nil
	}
	resumable, err := checkpoints.CanResumeCheckpoint(ctx, ExecutorCheckpointExpectation{
		RootMemberID:   rootContinuation.MemberID,
		SessionID:      pending.SessionID,
		CWD:            sess.CWD,
		WorkspaceCWD:   sess.CWD,
		Isolated:       false,
		GoalLeaseID:    pending.GoalLeaseID,
		ModelSelection: rootContinuation.ModelSelection,
		Limits:         rootContinuation.Limits,
		Capabilities:   pending.Capabilities,
	})
	if err != nil {
		return false, fmt.Errorf(
			"runs: validate executor checkpoint %q resumability: %w",
			rootContinuation.MemberID,
			err,
		)
	}
	return resumable, nil
}

func validateRecoveryContinuation(
	active run.Run,
	root run.Run,
	continuation Continuation,
) error {
	switch {
	case active.SessionID() != root.SessionID():
		return fmt.Errorf(
			"runs: validate recovery Run tree %q: Run %q belongs to Session %q, want %q",
			root.ID(),
			active.ID(),
			active.SessionID(),
			root.SessionID(),
		)
	case active.State() != run.Waiting:
		return fmt.Errorf(
			"runs: validate recovery Run tree %q: Run %q is %s, want waiting",
			root.ID(),
			active.ID(),
			active.State(),
		)
	case !active.Capabilities().Equal(root.Capabilities()):
		return fmt.Errorf(
			"runs: validate recovery Run tree %q: Run %q run capabilities differ from root admission",
			root.ID(),
			active.ID(),
		)
	}
	return validateContinuationRunFacts(root.ID(), active, continuation)
}

func indexRecoveryItems(items []transcript.Item) map[string]transcript.Item {
	indexed := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		indexed[item.ID] = item
	}
	return indexed
}

func validateRecoveryInterruptItems(
	tree recoveryRunTree,
	open []transcript.Interrupt,
	itemsByID map[string]transcript.Item,
) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(open))
	for _, request := range open {
		if _, duplicate := seen[request.ItemID]; duplicate {
			return nil, fmt.Errorf(
				"runs: validate recovery Run tree %q: duplicate interrupt Item %q",
				tree.root.ID(),
				request.ItemID,
			)
		}
		seen[request.ItemID] = struct{}{}
		if _, active := tree.runsByID[request.RunID]; !active {
			return nil, fmt.Errorf(
				"runs: validate recovery Run tree %q: interrupt Item %q belongs to non-active Run %q",
				tree.root.ID(),
				request.ItemID,
				request.RunID,
			)
		}
		item, found := itemsByID[request.ItemID]
		if !found ||
			item.SessionID != tree.root.SessionID() ||
			item.RunID != request.RunID ||
			item.Status != transcript.ItemRunning {
			return nil, fmt.Errorf(
				"runs: validate recovery Run tree %q: interrupt Item %q is not Running in Run %q",
				tree.root.ID(),
				request.ItemID,
				request.RunID,
			)
		}
		switch request.Kind {
		case interrupt.Approval:
			if request.Approval == nil ||
				request.Question != nil ||
				item.Kind != transcript.ToolCall ||
				item.Tool == nil ||
				!reflect.DeepEqual(*item.Tool, request.Approval.Tool) {
				return nil, fmt.Errorf(
					"runs: validate recovery Run tree %q: malformed approval Item %q",
					tree.root.ID(),
					request.ItemID,
				)
			}
		case interrupt.Question:
			if request.Question == nil ||
				request.Approval != nil ||
				item.Kind != transcript.QuestionItem ||
				item.Question == nil ||
				!reflect.DeepEqual(item.Question, request.Question) {
				return nil, fmt.Errorf(
					"runs: validate recovery Run tree %q: malformed question Item %q",
					tree.root.ID(),
					request.ItemID,
				)
			}
		default:
			return nil, fmt.Errorf(
				"runs: validate recovery Run tree %q: interrupt Item %q has unknown kind %d",
				tree.root.ID(),
				request.ItemID,
				request.Kind,
			)
		}
	}
	return seen, nil
}

func validateRecoveryRunningItems(
	tree recoveryRunTree,
	items []transcript.Item,
	interruptItems map[string]struct{},
) error {
	for _, item := range items {
		if _, active := tree.runsByID[item.RunID]; !active || item.Status != transcript.ItemRunning {
			continue
		}
		if _, belongsToInterrupt := interruptItems[item.ID]; !belongsToInterrupt {
			return fmt.Errorf(
				"runs: validate recovery Run tree %q: Running Item %q in Run %q has no matching interrupt",
				tree.root.ID(),
				item.ID,
				item.RunID,
			)
		}
	}
	return nil
}

func validateRecoveryContinuationTools(
	rootRunID string,
	continuation Continuation,
	itemsByID map[string]transcript.Item,
	claimedItems map[string]string,
) error {
	for _, drained := range continuation.DrainedTools {
		if err := claimRecoveryItem(rootRunID, continuation.RunID, drained.ItemID, "drained tool", claimedItems); err != nil {
			return err
		}
		item, found := itemsByID[drained.ItemID]
		if !found ||
			item.RunID != continuation.RunID ||
			item.Kind != transcript.ToolCall ||
			item.Status != transcript.ItemIncomplete ||
			item.Tool == nil ||
			item.Tool.Name != drained.Name ||
			item.Tool.Arguments.Canonical() != drained.Arguments ||
			item.Error != nil {
			return fmt.Errorf(
				"runs: validate recovery Run tree %q: malformed drained tool Item %q in Run %q",
				rootRunID,
				drained.ItemID,
				continuation.RunID,
			)
		}
	}
	for _, committed := range continuation.CommittedTools {
		if err := claimRecoveryItem(rootRunID, continuation.RunID, committed.ItemID, "committed tool", claimedItems); err != nil {
			return err
		}
		item, found := itemsByID[committed.ItemID]
		if !found ||
			item.RunID != continuation.RunID ||
			item.Kind != transcript.ToolCall ||
			item.Status != transcript.ItemIncomplete ||
			item.Tool == nil ||
			item.Tool.Name != committed.Name ||
			item.Tool.Arguments.Canonical() != committed.Arguments ||
			item.Error == nil ||
			!reflect.DeepEqual(*item.Error, committed.Failure) {
			return fmt.Errorf(
				"runs: validate recovery Run tree %q: malformed committed tool Item %q in Run %q",
				rootRunID,
				committed.ItemID,
				continuation.RunID,
			)
		}
	}
	return nil
}

func claimRecoveryItem(
	rootRunID string,
	runID string,
	itemID string,
	role string,
	claimedItems map[string]string,
) error {
	if previous, duplicate := claimedItems[itemID]; duplicate {
		return fmt.Errorf(
			"runs: validate recovery Run tree %q: Item %q in Run %q is both %s and %s",
			rootRunID,
			itemID,
			runID,
			previous,
			role,
		)
	}
	claimedItems[itemID] = role
	return nil
}
