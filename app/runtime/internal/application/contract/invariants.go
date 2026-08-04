// Package contract names the runtime's cross-resource invariants and says which
// transaction boundaries are responsible for keeping each one.
//
// It lives in the application ring on purpose (vNext plan D3). These invariants
// span runs, interrupts, items and the store, so the delivery ring — which must
// stay wire-only — has no business naming them; and if the type lived in
// delivery, registering here would make the application ring depend OUTWARD,
// which the dependency rule forbids.
//
// What this package does NOT do is enforce anything. A [SystemInvariantSpec]
// gives an invariant a stable name and points at the code responsible for it. The
// alternative — a Validate() that reaches into a repository to check a
// cross-resource fact — is exactly what contract §11.2 rules out: it would make a
// DTO validator depend on storage, and it would pretend a frame-local check can
// see the whole system. Verification is a cross-projection integration fixture.
package contract

import (
	"errors"
	"fmt"
	"slices"
)

// TransactionBoundary is the atomic write whose job it is to maintain an
// invariant. The value is the code path, not a table: an invariant is kept by a
// transition, and a reader needs to know which one to go read.
type TransactionBoundary string

const (
	// BoundaryRunAdmission is the admission write that lets a Run exist.
	BoundaryRunAdmission TransactionBoundary = "runs.admission"
	// BoundarySegmentOpening is the one transaction that admits or resumes a Run
	// and lands its opening projections.
	BoundarySegmentOpening TransactionBoundary = "runsegment.opening"
	// BoundarySegmentEvent is the one transaction per run event: the open
	// interrupt, the transcript items, and the lifecycle transition together.
	BoundarySegmentEvent TransactionBoundary = "runsegment.event"
	// BoundaryWaitingSubtreeCancellation is the transaction that replaces one
	// parked child subtree while preserving or resuming the surviving tree.
	BoundaryWaitingSubtreeCancellation TransactionBoundary = "runsegment.waiting_subtree_cancel"
	// BoundaryRunRecovery is the boot sweep that converges Runs whose executor
	// vanished while the process was down.
	BoundaryRunRecovery TransactionBoundary = "runs.recovery"
	// BoundaryParkedTermination is the online cancellation or executor-loss write
	// that ends every member of a parked Run tree and consumes its hand-off.
	BoundaryParkedTermination TransactionBoundary = "sessions.parked_terminal"
	// BoundarySessionRollback truncates a session's history at a run boundary.
	BoundarySessionRollback TransactionBoundary = "sessions.rollback"
	// BoundarySessionDelete removes a session and everything it owns.
	BoundarySessionDelete TransactionBoundary = "sessions.delete"
	// BoundarySessionImport restores a session from a portable artifact.
	BoundarySessionImport TransactionBoundary = "sessions.import"
	// BoundaryGoalLifecycle is a goal's compare-and-swap lifecycle transition.
	BoundaryGoalLifecycle TransactionBoundary = "goals.lifecycle"
)

// SystemInvariantSpec names one invariant and the boundaries that maintain it.
type SystemInvariantSpec struct {
	// Key is the stable name a fixture registers coverage against.
	Key string
	// Why states what breaks when the invariant does. An invariant nobody can
	// explain is one nobody will preserve during a refactor.
	Why string
	// Boundaries are the transactions responsible. More than one means the
	// invariant has several ways to be broken, and all of them must hold it.
	Boundaries []TransactionBoundary
}

// SystemInvariants is the declared set. Adding a cross-resource invariant means
// adding it here; contract §11.4 gate 8 refuses one with no integration fixture,
// and a fixture that covers no declared key.
func SystemInvariants() []SystemInvariantSpec {
	out := slices.Clone(systemInvariants)
	for index := range out {
		out[index].Boundaries = slices.Clone(out[index].Boundaries)
	}
	return out
}

var systemInvariants = []SystemInvariantSpec{{
	Key: "session_has_at_most_one_open_run",
	Why: "Two concurrently-appending Runs would interleave one session's history, " +
		"and the second would read a transcript the first had not finished writing.",
	Boundaries: []TransactionBoundary{BoundaryRunAdmission, BoundarySegmentOpening},
}, {
	Key: "terminal_run_explains_how_it_ended",
	Why: "A Run row that claims a terminal state without the outcome — and, when " +
		"that outcome is a failure, the failure itself — cannot answer why the run " +
		"ended, and no later write will supply it.",
	Boundaries: []TransactionBoundary{BoundarySegmentEvent, BoundaryRunRecovery, BoundarySessionImport},
}, {
	Key: "run_protocol_profile_is_immutable",
	Why: "A Run publishes under the contract it was created with. If a later " +
		"segment could restate it, a subscriber that reconnected on the strength of " +
		"the first answer would be folding a stream whose rules had changed under it.",
	Boundaries: []TransactionBoundary{BoundaryRunAdmission},
}, {
	Key: "parked_tree_has_exactly_one_open_interrupt_set",
	Why: "A Run tree parked without one complete pending set cannot be resumed " +
		"atomically; clients would observe only part of a barrier that can never move.",
	Boundaries: []TransactionBoundary{BoundarySegmentEvent, BoundaryRunRecovery},
}, {
	Key: "parked_continuation_matches_run_facts",
	Why: "A continuation is a hand-off of the admitted Run, not a second author. " +
		"If its model, cumulative accounting, limits, lineage, creation time, goal " +
		"lease or protocol contract differs, resume or teardown would rewrite history.",
	Boundaries: []TransactionBoundary{
		BoundarySegmentOpening,
		BoundarySegmentEvent,
		BoundaryWaitingSubtreeCancellation,
		BoundaryRunRecovery,
		BoundaryParkedTermination,
	},
}, {
	Key: "dropped_run_leaves_nothing_behind",
	Why: "A dropped Run's items, interrupts, checkpoints and admission slot must " +
		"go with it, or the session keeps an invisible run holding its only slot.",
	Boundaries: []TransactionBoundary{BoundarySessionRollback, BoundarySessionDelete},
}, {
	Key: "imported_session_keeps_its_identity",
	Why: "Import is restore, not copy: an artifact must come back under the id it " +
		"was exported with, or its runs and items reference a session that is gone.",
	Boundaries: []TransactionBoundary{BoundarySessionImport},
}, {
	Key: "goal_never_outlives_its_session",
	Why: "A goal is session-owned. One that survives its session's deletion or " +
		"rollback would keep driving runs toward an objective nobody can see or stop.",
	Boundaries: []TransactionBoundary{BoundaryGoalLifecycle, BoundarySessionDelete, BoundarySessionRollback},
}}

// Validate rejects a declaration that cannot be acted on: an invariant with no
// responsible boundary is a wish, and a duplicate key would let two fixtures each
// think it covered the other's invariant.
func Validate() error {
	seen := make(map[string]bool, len(systemInvariants))
	for _, spec := range systemInvariants {
		switch {
		case spec.Key == "":
			return errors.New("system invariant: key is required")
		case seen[spec.Key]:
			return fmt.Errorf("system invariant %q is declared twice", spec.Key)
		case spec.Why == "":
			return fmt.Errorf("system invariant %q: state what breaks without it", spec.Key)
		case len(spec.Boundaries) == 0:
			return fmt.Errorf("system invariant %q: no transaction is responsible for it", spec.Key)
		}
		seen[spec.Key] = true
	}
	return nil
}
