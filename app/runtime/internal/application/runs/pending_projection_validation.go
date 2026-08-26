package runs

import (
	"fmt"
	"reflect"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/interrupt"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

func indexPendingItems(items []transcript.Item) map[string]transcript.Item {
	indexed := make(map[string]transcript.Item, len(items))
	for _, item := range items {
		indexed[item.ID()] = item
	}
	return indexed
}

func validatePendingInterruptItems(
	rootRunID string,
	sessionID string,
	activeRuns map[string]run.Run,
	open []transcript.Interrupt,
	itemsByID map[string]transcript.Item,
) (map[string]struct{}, error) {
	seen := make(map[string]struct{}, len(open))
	for _, request := range open {
		if _, duplicate := seen[request.ItemID]; duplicate {
			return nil, fmt.Errorf(
				"runs: validate parked Run tree %q: duplicate interrupt Item %q",
				rootRunID,
				request.ItemID,
			)
		}
		seen[request.ItemID] = struct{}{}
		if _, active := activeRuns[request.RunID]; !active {
			return nil, fmt.Errorf(
				"runs: validate parked Run tree %q: interrupt Item %q belongs to non-active Run %q",
				rootRunID,
				request.ItemID,
				request.RunID,
			)
		}
		item, found := itemsByID[request.ItemID]
		if !found ||
			item.SessionID() != sessionID ||
			item.RunID() != request.RunID ||
			!item.OccurredAt().Equal(request.ItemOccurredAt) {
			return nil, fmt.Errorf(
				"runs: validate parked Run tree %q: interrupt Item %q is not the exact Item owned by Run %q",
				rootRunID,
				request.ItemID,
				request.RunID,
			)
		}
		switch request.Kind {
		case interrupt.Approval:
			invocation, present := item.ToolInvocation()
			if request.Approval == nil ||
				request.Question != nil ||
				item.Kind() != transcript.ToolCall ||
				item.Status() != transcript.ItemRunning ||
				item.ApprovalDecision() != "" ||
				!present ||
				!reflect.DeepEqual(invocation, request.Approval.Tool) {
				return nil, fmt.Errorf(
					"runs: validate parked Run tree %q: malformed approval Item %q",
					rootRunID,
					request.ItemID,
				)
			}
		case interrupt.Question:
			question, present := item.Question()
			if request.Question == nil ||
				request.Approval != nil ||
				item.Kind() != transcript.QuestionItem ||
				item.Status() != transcript.ItemCompleted ||
				!present ||
				!reflect.DeepEqual(question, *request.Question) {
				return nil, fmt.Errorf(
					"runs: validate parked Run tree %q: malformed question Item %q",
					rootRunID,
					request.ItemID,
				)
			}
		default:
			return nil, fmt.Errorf(
				"runs: validate parked Run tree %q: interrupt Item %q has unknown kind %q",
				rootRunID,
				request.ItemID,
				request.Kind,
			)
		}
	}
	return seen, nil
}

func validatePendingRunningItems(
	rootRunID string,
	activeRuns map[string]run.Run,
	items []transcript.Item,
	interruptItems map[string]struct{},
	drainedItems map[string]struct{},
) error {
	for _, item := range items {
		if _, active := activeRuns[item.RunID()]; !active || item.Status() != transcript.ItemRunning {
			continue
		}
		_, belongsToInterrupt := interruptItems[item.ID()]
		_, belongsToDrainedTool := drainedItems[item.ID()]
		if !belongsToInterrupt && !belongsToDrainedTool {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: Running Item %q in Run %q has no matching interrupt or drained Tool",
				rootRunID,
				item.ID(),
				item.RunID(),
			)
		}
	}
	return nil
}

func validatePendingContinuationTools(
	rootRunID string,
	sessionID string,
	continuation Continuation,
	itemsByID map[string]transcript.Item,
	claimedItems map[string]string,
) error {
	for _, drained := range continuation.DrainedTools {
		if err := claimPendingItem(rootRunID, continuation.RunID, drained.ItemID, "drained tool", claimedItems); err != nil {
			return err
		}
		item, found := itemsByID[drained.ItemID]
		invocation, hasInvocation := item.ToolInvocation()
		_, hasFailure := item.Failure()
		if !found ||
			item.SessionID() != sessionID ||
			item.RunID() != continuation.RunID ||
			!item.OccurredAt().Equal(drained.ItemOccurredAt) ||
			item.Kind() != transcript.ToolCall ||
			item.Status() != transcript.ItemRunning ||
			!hasInvocation ||
			invocation.Name != drained.Name ||
			invocation.Arguments.Canonical() != drained.Arguments ||
			hasFailure {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: malformed drained tool Item %q in Run %q",
				rootRunID,
				drained.ItemID,
				continuation.RunID,
			)
		}
	}
	for _, committed := range continuation.CommittedTools {
		if err := claimPendingItem(rootRunID, continuation.RunID, committed.ItemID, "committed tool", claimedItems); err != nil {
			return err
		}
		item, found := itemsByID[committed.ItemID]
		invocation, hasInvocation := item.ToolInvocation()
		failure, hasFailure := item.Failure()
		if !found ||
			item.SessionID() != sessionID ||
			item.RunID() != continuation.RunID ||
			item.Kind() != transcript.ToolCall ||
			item.Status() != transcript.ItemIncomplete ||
			!hasInvocation ||
			invocation.Name != committed.Name ||
			invocation.Arguments.Canonical() != committed.Arguments ||
			!hasFailure ||
			!reflect.DeepEqual(failure, committed.Failure) {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: malformed committed tool Item %q in Run %q",
				rootRunID,
				committed.ItemID,
				continuation.RunID,
			)
		}
	}
	return nil
}

func claimPendingItem(
	rootRunID string,
	runID string,
	itemID string,
	role string,
	claimedItems map[string]string,
) error {
	if previous, duplicate := claimedItems[itemID]; duplicate {
		return fmt.Errorf(
			"runs: validate parked Run tree %q: Item %q in Run %q is both %s and %s",
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
