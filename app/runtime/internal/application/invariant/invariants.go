// Package invariant names the runtime's cross-resource invariants and says which
// transaction boundaries are responsible for keeping each one.
//
// These invariants span Runs, Interrupts, Items, and persistence, so the use-case
// layer must name them independently of any external contract.
//
// What this package does NOT do is enforce anything. A [Spec]
// gives an invariant a stable name and points at the code responsible for it. The
// alternative — a Validate() that reaches into a repository to check a
// cross-resource fact — is exactly what contract §11.2 rules out: it would make a
// local value validator depend on storage, and it would pretend a value-local check can
// see the whole system. Verification is a cross-projection integration fixture.
package invariant

import (
	"slices"
)

// Boundary is the atomic write whose job it is to maintain an
// invariant. The value is the code path, not a table: an invariant is kept by a
// transition, and a reader needs to know which one to go read.
type Boundary string

const (
	// BoundaryRunAdmission is the admission write that lets a Run exist.
	BoundaryRunAdmission Boundary = "runs.admission"
	// BoundarySegmentOpening is the one transaction that admits or resumes a Run
	// and lands its opening projections.
	BoundarySegmentOpening Boundary = "runsegment.opening"
	// BoundarySegmentEvent is the one transaction per run event: the open
	// interrupt, the transcript items, and the lifecycle transition together.
	BoundarySegmentEvent Boundary = "runsegment.event"
	// BoundaryWaitingSubtreeCancellation is the transaction that replaces one
	// parked child subtree while preserving or resuming the surviving tree.
	BoundaryWaitingSubtreeCancellation Boundary = "runsegment.waiting_subtree_cancel"
	// BoundaryRunRecovery is the boot sweep that converges Runs whose executor
	// vanished while the process was down.
	BoundaryRunRecovery Boundary = "runs.recovery"
	// BoundaryParkedTermination is the online cancellation or executor-loss write
	// that ends every member of a parked Run tree and consumes its hand-off.
	BoundaryParkedTermination Boundary = "sessions.parked_terminal"
	// BoundarySessionRollback truncates a session's history at a run boundary.
	BoundarySessionRollback Boundary = "sessions.rollback"
	// BoundarySessionDelete removes a session and everything it owns.
	BoundarySessionDelete Boundary = "sessions.delete"
	// BoundarySessionImport restores a session from a portable artifact.
	BoundarySessionImport Boundary = "sessions.import"
	// BoundaryGoalLifecycle is a goal's compare-and-swap lifecycle transition.
	BoundaryGoalLifecycle Boundary = "goals.lifecycle"
)

// Spec names one invariant and the boundaries that maintain it.
type Spec struct {
	// Key is the stable name a fixture registers coverage against.
	Key string
	// Why states what breaks when the invariant does. An invariant nobody can
	// explain is one nobody will preserve during a refactor.
	Why string
	// Boundaries are the transactions responsible. More than one means the
	// invariant has several ways to be broken, and all of them must hold it.
	Boundaries []Boundary
}

// All is the declared set. Adding a cross-resource invariant means
// adding it here; contract §11.4 gate 8 refuses one with no integration fixture,
// and a fixture that covers no declared key.
func All() []Spec {
	out := slices.Clone(specs)
	for index := range out {
		out[index].Boundaries = slices.Clone(out[index].Boundaries)
	}
	return out
}

var specs = []Spec{{
	Key: "session_has_at_most_one_open_run",
	Why: "Two concurrently-appending Runs would interleave one session's history, " +
		"and the second would read a transcript the first had not finished writing.",
	Boundaries: []Boundary{BoundaryRunAdmission, BoundarySegmentOpening},
}, {
	Key: "terminal_run_explains_how_it_ended",
	Why: "A Run row that claims a terminal state without the outcome — and, when " +
		"that outcome is a failure, the failure itself — cannot answer why the run " +
		"ended, and no later write will supply it.",
	Boundaries: []Boundary{BoundarySegmentEvent, BoundaryRunRecovery, BoundarySessionImport},
}, {
	Key: "run_capabilities_are_immutable",
	Why: "A Run may exercise only the optional behavior enabled at admission. If " +
		"a later segment could restate it, continuation and subscription checks would " +
		"no longer describe the same Run.",
	Boundaries: []Boundary{BoundaryRunAdmission},
}, {
	Key: "parked_tree_has_exactly_one_open_interrupt_set",
	Why: "A Run tree parked without one complete pending set cannot be resumed " +
		"atomically; clients would observe only part of a barrier that can never move.",
	Boundaries: []Boundary{BoundarySegmentEvent, BoundaryRunRecovery},
}, {
	Key: "parked_continuation_matches_run_facts",
	Why: "A continuation is a hand-off of the admitted Run, not a second author. " +
		"If its model, cumulative accounting, limits, lineage, creation time, goal " +
		"lease or capabilities differ, resume or teardown would rewrite history.",
	Boundaries: []Boundary{
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
	Boundaries: []Boundary{BoundarySessionRollback, BoundarySessionDelete},
}, {
	Key: "imported_session_keeps_its_identity",
	Why: "Import is restore, not copy: an artifact must come back under the id it " +
		"was exported with, or its runs and items reference a session that is gone.",
	Boundaries: []Boundary{BoundarySessionImport},
}, {
	Key: "goal_never_outlives_its_session",
	Why: "A goal is session-owned. One that survives its session's deletion or " +
		"rollback would keep driving runs toward an objective nobody can see or stop.",
	Boundaries: []Boundary{BoundaryGoalLifecycle, BoundarySessionDelete, BoundarySessionRollback},
}}
