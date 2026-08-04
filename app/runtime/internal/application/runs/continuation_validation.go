package runs

import (
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/transcript"
)

// validatePendingRunTree cross-checks the complete active Run tree before a
// continuation can drive an executor or a lifecycle transformation. Terminal
// descendants may remain in the historical tree, but every non-terminal Run is
// Interrupted and represented by exactly one canonical continuation.
func validatePendingRunTree(pending interrupts.Pending, values []transcript.Run) error {
	if err := pending.Validate(); err != nil {
		return fmt.Errorf("runs: validate parked Run tree %q: %w", pending.RootRunID, err)
	}
	all := make(map[string]transcript.Run, len(values))
	active := make(map[string]transcript.Run, len(pending.Continuations))
	for index, run := range values {
		if err := run.Validate(); err != nil {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: Run[%d] %q: %w",
				pending.RootRunID,
				index,
				run.ID,
				err,
			)
		}
		if _, duplicate := all[run.ID]; duplicate {
			return fmt.Errorf("runs: validate parked Run tree %q: duplicate Run %q", pending.RootRunID, run.ID)
		}
		all[run.ID] = run
		if !run.State.IsTerminal() {
			active[run.ID] = run
		}
	}
	root, found := all[pending.RootRunID]
	if !found || !root.Lineage().IsRoot() {
		return fmt.Errorf("runs: validate parked Run tree %q: root Run is missing", pending.RootRunID)
	}
	if root.SessionID != pending.SessionID || root.State != execution.Interrupted {
		return fmt.Errorf("runs: validate parked Run tree %q: root Run scope or state differs from Pending", pending.RootRunID)
	}
	if root.GoalLeaseID != pending.GoalLeaseID {
		return fmt.Errorf("runs: validate parked Run tree %q: root Run goal lease differs from Pending", pending.RootRunID)
	}
	if !root.Capabilities.Equal(pending.Capabilities) {
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
		run, found := active[continuation.RunID]
		if !found || run.SessionID != pending.SessionID || run.State != execution.Interrupted {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: continuation Run %q is not active and interrupted",
				pending.RootRunID,
				continuation.RunID,
			)
		}
		if err := validateContinuationRunFacts(pending.RootRunID, run, continuation); err != nil {
			return err
		}
		if !run.Capabilities.Equal(root.Capabilities) {
			return fmt.Errorf(
				"runs: validate parked Run tree %q: Run %q run capabilities differ from root admission",
				pending.RootRunID,
				run.ID,
			)
		}
	}
	return nil
}

// validateContinuationRunFacts proves that a durable continuation is a hand-off
// of run, not a second author for immutable admission or cumulative accounting.
// Lifecycle callers separately validate state, tree coverage, Pending ownership,
// and the root-owned protocol/Goal facts that do not live on each continuation.
func validateContinuationRunFacts(
	rootRunID string,
	run transcript.Run,
	continuation interrupts.Continuation,
) error {
	switch {
	case run.ID != continuation.RunID:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q differs from continuation owner %q",
			rootRunID,
			run.ID,
			continuation.RunID,
		)
	case run.ModelSelection != continuation.ModelSelection:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q admission model %q/%q differs from continuation model %q/%q",
			rootRunID,
			run.ID,
			run.ModelSelection.Provider(),
			run.ModelSelection.Model(),
			continuation.ModelSelection.Provider(),
			continuation.ModelSelection.Model(),
		)
	case !run.Metrics.Equal(continuation.Metrics):
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q cumulative metrics differ from its continuation",
			rootRunID,
			run.ID,
		)
	case run.Limits != continuation.Limits:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q frozen limits differ from its continuation",
			rootRunID,
			run.ID,
		)
	case !run.CreatedAt.Equal(continuation.RunCreatedAt):
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q and continuation creation times differ",
			rootRunID,
			run.ID,
		)
	case run.Lineage() != continuation.Lineage:
		return fmt.Errorf(
			"runs: validate Run tree %q: Run %q lineage differs from its continuation",
			rootRunID,
			run.ID,
		)
	default:
		return nil
	}
}
