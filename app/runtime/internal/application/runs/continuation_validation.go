package runs

import (
	"fmt"

	rundomain "github.com/Tangerg/scope/app/runtime/internal/domain/run"
	"github.com/Tangerg/scope/app/runtime/internal/domain/transcript"
)

// ValidateProjection proves that one Pending barrier is the complete owner of
// the durable Run and transcript facts a mounted client will receive. Values
// may contain the Session's full history; only this barrier's Run tree is
// selected, while every referenced Item is resolved from the complete Item set.
func (p Pending) ValidateProjection(values []rundomain.Run, items []transcript.Item) error {
	treeRuns := make([]rundomain.Run, 0, len(p.Continuations))
	for _, value := range values {
		if value.Lineage().TreeRootID(value.ID()) == p.RootRunID {
			treeRuns = append(treeRuns, value)
		}
	}
	if err := validatePendingRunTree(p, treeRuns); err != nil {
		return err
	}

	active := make(map[string]rundomain.Run, len(p.Continuations))
	for _, value := range treeRuns {
		if !value.State().IsTerminal() {
			active[value.ID()] = value
		}
	}
	itemsByID := indexPendingItems(items)
	interruptItems, err := validatePendingInterruptItems(
		p.RootRunID,
		p.SessionID,
		active,
		p.Interrupts,
		itemsByID,
	)
	if err != nil {
		return err
	}
	drainedItems := make(map[string]struct{})
	for _, continuation := range p.Continuations {
		for _, drained := range continuation.DrainedTools {
			drainedItems[drained.ItemID] = struct{}{}
		}
	}
	if err := validatePendingRunningItems(
		p.RootRunID,
		active,
		items,
		interruptItems,
		drainedItems,
	); err != nil {
		return err
	}
	claimedItems := make(map[string]string, len(interruptItems))
	for itemID := range interruptItems {
		claimedItems[itemID] = "interrupt"
	}
	for _, continuation := range p.Continuations {
		if err := validatePendingContinuationTools(
			p.RootRunID,
			p.SessionID,
			continuation,
			itemsByID,
			claimedItems,
		); err != nil {
			return err
		}
	}
	return nil
}

// validatePendingRunTree cross-checks the complete active Run tree before a
// continuation can drive an executor or a lifecycle transformation. Terminal
// descendants may remain in the historical tree, but every non-terminal Run is
// Waiting and represented by exactly one canonical continuation.
func validatePendingRunTree(pending Pending, values []rundomain.Run) error {
	if err := pending.Validate(); err != nil {
		return fmt.Errorf("runs: validate parked Run tree %q: %w", pending.RootRunID, err)
	}
	all := make(map[string]rundomain.Run, len(values))
	active := make(map[string]rundomain.Run, len(pending.Continuations))
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: Run[%d] %q: %w",
				pending.RootRunID,
				index,
				value.ID(),
				err,
			)
		}
		if _, duplicate := all[value.ID()]; duplicate {
			return fmt.Errorf("runs: validate parked Run tree %q: duplicate Run %q", pending.RootRunID, value.ID())
		}
		all[value.ID()] = value
		if !value.State().IsTerminal() {
			active[value.ID()] = value
		}
	}
	root, found := all[pending.RootRunID]
	if !found || !root.Lineage().IsRoot() {
		return fmt.Errorf("runs: validate parked Run tree %q: root Run is missing", pending.RootRunID)
	}
	if root.SessionID() != pending.SessionID || root.State() != rundomain.Waiting {
		return fmt.Errorf("runs: validate parked Run tree %q: root Run scope or state differs from Pending", pending.RootRunID)
	}
	if root.GoalIncarnationID() != pending.GoalIncarnationID {
		return fmt.Errorf("runs: validate parked Run tree %q: root Run goal incarnation differs from Pending", pending.RootRunID)
	}
	if !root.Capabilities().Equal(pending.Capabilities) {
		return fmt.Errorf("runs: validate parked Run tree %q: root Run run capabilities differ from Pending", pending.RootRunID)
	}
	if len(active) != len(pending.Continuations) {
		return fmt.Errorf(
			"runs: validate parked Run tree %q: %d continuations do not cover %d active Runs",
			pending.RootRunID,
			len(pending.Continuations),
			len(active),
		)
	}
	for _, continuation := range pending.Continuations {
		value, found := active[continuation.RunID]
		if !found || value.SessionID() != pending.SessionID || value.State() != rundomain.Waiting {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: continuation Run %q is not active and waiting",
				pending.RootRunID,
				continuation.RunID,
			)
		}
		if err := validateContinuationRunFacts(pending.RootRunID, value, continuation); err != nil {
			return err
		}
		if !value.Capabilities().Equal(root.Capabilities()) {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: Run %q run capabilities differ from root admission",
				pending.RootRunID,
				value.ID(),
			)
		}
	}
	return nil
}

// validateContinuationRunFacts proves that a durable continuation is a hand-off
// of run, not a second author for immutable admission or cumulative accounting.
// Lifecycle callers separately validate state, tree coverage, Pending ownership,
// and root-owned capability and Goal facts that do not live on each continuation.
func validateContinuationRunFacts(
	rootRunID string,
	value rundomain.Run,
	continuation Continuation,
) error {
	switch {
	case value.ID() != continuation.RunID:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q differs from continuation owner %q",
			rootRunID,
			value.ID(),
			continuation.RunID,
		)
	case value.ModelSelection() != continuation.ModelSelection:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q admission model %q/%q differs from continuation model %q/%q",
			rootRunID,
			value.ID(),
			value.ModelSelection().Provider(),
			value.ModelSelection().Model(),
			continuation.ModelSelection.Provider(),
			continuation.ModelSelection.Model(),
		)
	case !value.Metrics().Equal(continuation.Metrics):
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q cumulative metrics differ from its continuation",
			rootRunID,
			value.ID(),
		)
	case value.Limits() != continuation.Limits:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q frozen limits differ from its continuation",
			rootRunID,
			value.ID(),
		)
	case !value.CreatedAt().Equal(continuation.RunCreatedAt):
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q and continuation creation times differ",
			rootRunID,
			value.ID(),
		)
	case value.Lineage() != continuation.Lineage:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q lineage differs from its continuation",
			rootRunID,
			value.ID(),
		)
	default:
		return nil
	}
}
