package sqlite

import (
	"context"
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// validateParkedTree checks the complete durable barrier before boot keeps a
// Run tree resumable. Pending is root-owned, so its continuations must cover
// every non-terminal member exactly once; validating only the root would turn
// every surviving child into a false orphan after restart.
//
// An impossible partial application write is corruption and fails startup. A
// missing or executor-incompatible process state is an external-resource
// loss and returns resumable=false so reconciliation can recover the whole tree
// as run_lost.
func (s *RunStore) validateParkedTree(
	ctx context.Context,
	tree nonTerminalRunTree,
	pending interrupts.Pending,
	validateProcess ResumableProcessValidator,
) (bool, error) {
	if err := pending.Validate(); err != nil {
		return false, fmt.Errorf(
			"sqlite: validate parked Run tree %q Pending: %w",
			tree.root.ID,
			err,
		)
	}
	if pending.RootRunID != tree.root.ID ||
		pending.SessionID != tree.root.SessionID {
		return false, fmt.Errorf(
			"sqlite: validate parked Run tree %q: Pending identity is %q/%q, want %q/%q",
			tree.root.ID,
			pending.SessionID,
			pending.RootRunID,
			tree.root.SessionID,
			tree.root.ID,
		)
	}
	if len(pending.Continuations) != len(tree.runsByID) {
		return false, fmt.Errorf(
			"sqlite: validate parked Run tree %q: Pending has %d continuations, want %d active Runs",
			tree.root.ID,
			len(pending.Continuations),
			len(tree.runsByID),
		)
	}
	for _, continuation := range pending.Continuations {
		active, found := tree.runsByID[continuation.RunID]
		if !found {
			return false, fmt.Errorf(
				"sqlite: validate parked Run tree %q: continuation names non-active Run %q",
				tree.root.ID,
				continuation.RunID,
			)
		}
		if err := validateParkedContinuation(active, tree.root, continuation); err != nil {
			return false, err
		}
	}

	items, err := NewTranscriptStore(s.db).List(ctx, tree.root.SessionID)
	if err != nil {
		return false, fmt.Errorf(
			"sqlite: validate parked Run tree %q transcript: %w",
			tree.root.ID,
			err,
		)
	}
	itemsByID := indexTranscriptItems(items)
	interruptItems, err := validatePendingInterruptItems(
		tree,
		pending.Interrupts,
		itemsByID,
	)
	if err != nil {
		return false, err
	}
	if err := validateRunningInterruptItems(tree, items, interruptItems); err != nil {
		return false, err
	}
	claimedItems := make(map[string]string, len(interruptItems))
	for itemID := range interruptItems {
		claimedItems[itemID] = "interrupt"
	}
	for _, continuation := range pending.Continuations {
		if err := validateContinuationTools(
			tree.root.ID,
			continuation,
			itemsByID,
			claimedItems,
		); err != nil {
			return false, err
		}
	}

	rootContinuation, _ := pending.RootContinuation()
	return s.hasResumableProcess(
		ctx,
		rootContinuation.ProcessID,
		validateProcess,
	)
}

func validateParkedContinuation(
	active transcript.Run,
	root transcript.Run,
	continuation interrupts.Continuation,
) error {
	switch {
	case active.SessionID != root.SessionID:
		return fmt.Errorf(
			"sqlite: validate parked Run tree %q: Run %q belongs to Session %q, want %q",
			root.ID,
			active.ID,
			active.SessionID,
			root.SessionID,
		)
	case active.State != execution.Interrupted:
		return fmt.Errorf(
			"sqlite: validate parked Run tree %q: Run %q is %s, want interrupted",
			root.ID,
			active.ID,
			active.State,
		)
	case active.ModelSelection != continuation.ModelSelection:
		return fmt.Errorf(
			"sqlite: validate parked Run tree %q: Run %q admission model %q/%q differs from continuation model %q/%q",
			root.ID,
			active.ID,
			active.ModelSelection.Provider(),
			active.ModelSelection.Model(),
			continuation.ModelSelection.Provider(),
			continuation.ModelSelection.Model(),
		)
	case !active.CreatedAt.Equal(continuation.RunCreatedAt):
		return fmt.Errorf(
			"sqlite: validate parked Run tree %q: Run %q and continuation creation times differ",
			root.ID,
			active.ID,
		)
	case active.Lineage() != continuation.Lineage:
		return fmt.Errorf(
			"sqlite: validate parked Run tree %q: Run %q lineage differs from its continuation",
			root.ID,
			active.ID,
		)
	default:
		return nil
	}
}

func indexTranscriptItems(items []transcript.Item) map[string]transcript.Item {
	indexed := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		indexed[item.ID] = item
	}
	return indexed
}

func validatePendingInterruptItems(
	tree nonTerminalRunTree,
	open []transcript.Interrupt,
	itemsByID map[string]transcript.Item,
) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(open))
	for _, interrupt := range open {
		if _, duplicate := seen[interrupt.ItemID]; duplicate {
			return nil, fmt.Errorf(
				"sqlite: validate parked Run tree %q: duplicate interrupt Item %q",
				tree.root.ID,
				interrupt.ItemID,
			)
		}
		seen[interrupt.ItemID] = struct{}{}
		if _, active := tree.runsByID[interrupt.RunID]; !active {
			return nil, fmt.Errorf(
				"sqlite: validate parked Run tree %q: interrupt Item %q belongs to non-active Run %q",
				tree.root.ID,
				interrupt.ItemID,
				interrupt.RunID,
			)
		}
		item, found := itemsByID[interrupt.ItemID]
		if !found ||
			item.SessionID != tree.root.SessionID ||
			item.RunID != interrupt.RunID ||
			item.Status != transcript.ItemRunning {
			return nil, fmt.Errorf(
				"sqlite: validate parked Run tree %q: interrupt Item %q is not Running in Run %q",
				tree.root.ID,
				interrupt.ItemID,
				interrupt.RunID,
			)
		}
		switch interrupt.Kind {
		case execution.ApprovalInterrupt:
			if interrupt.Approval == nil ||
				interrupt.Question != nil ||
				item.Kind != transcript.ToolCall ||
				item.Tool == nil ||
				!reflect.DeepEqual(*item.Tool, interrupt.Approval.Tool) {
				return nil, fmt.Errorf(
					"sqlite: validate parked Run tree %q: malformed approval Item %q",
					tree.root.ID,
					interrupt.ItemID,
				)
			}
		case execution.QuestionInterrupt:
			if interrupt.Question == nil ||
				interrupt.Approval != nil ||
				item.Kind != transcript.QuestionItem ||
				item.Question == nil ||
				!reflect.DeepEqual(item.Question, interrupt.Question) {
				return nil, fmt.Errorf(
					"sqlite: validate parked Run tree %q: malformed question Item %q",
					tree.root.ID,
					interrupt.ItemID,
				)
			}
		default:
			return nil, fmt.Errorf(
				"sqlite: validate parked Run tree %q: interrupt Item %q has unknown kind %d",
				tree.root.ID,
				interrupt.ItemID,
				interrupt.Kind,
			)
		}
	}
	return seen, nil
}

func validateRunningInterruptItems(
	tree nonTerminalRunTree,
	items []transcript.Item,
	interruptItems map[string]struct{},
) error {
	for _, item := range items {
		if _, active := tree.runsByID[item.RunID]; !active ||
			item.Status != transcript.ItemRunning {
			continue
		}
		if _, belongsToInterrupt := interruptItems[item.ID]; !belongsToInterrupt {
			return fmt.Errorf(
				"sqlite: validate parked Run tree %q: Running Item %q in Run %q has no matching interrupt",
				tree.root.ID,
				item.ID,
				item.RunID,
			)
		}
	}
	return nil
}

func validateContinuationTools(
	rootRunID string,
	continuation interrupts.Continuation,
	itemsByID map[string]transcript.Item,
	claimedItems map[string]string,
) error {
	for _, drained := range continuation.DrainedTools {
		if err := claimContinuationItem(
			rootRunID,
			continuation.RunID,
			drained.ItemID,
			"drained tool",
			claimedItems,
		); err != nil {
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
				"sqlite: validate parked Run tree %q: malformed drained tool Item %q in Run %q",
				rootRunID,
				drained.ItemID,
				continuation.RunID,
			)
		}
	}
	for _, committed := range continuation.CommittedTools {
		if err := claimContinuationItem(
			rootRunID,
			continuation.RunID,
			committed.ItemID,
			"committed tool",
			claimedItems,
		); err != nil {
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
			!reflect.DeepEqual(*item.Error, committed.Problem) {
			return fmt.Errorf(
				"sqlite: validate parked Run tree %q: malformed committed tool Item %q in Run %q",
				rootRunID,
				committed.ItemID,
				continuation.RunID,
			)
		}
	}
	return nil
}

func claimContinuationItem(
	rootRunID string,
	runID string,
	itemID string,
	role string,
	claimedItems map[string]string,
) error {
	if previous, duplicate := claimedItems[itemID]; duplicate {
		return fmt.Errorf(
			"sqlite: validate parked Run tree %q: Item %q in Run %q is both %s and %s",
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
