package main

// systemInvariants returns contract-generation input, not a Runtime package.
// The named transactions remain Application behavior; keeping their published
// catalog beside the manifest generator prevents documentation metadata from
// becoming a production Application dependency.
func systemInvariants() []invariantEntry {
	return []invariantEntry{{
		Key: "session_has_at_most_one_open_run",
		Why: "Two concurrently-appending Runs would interleave one session's history, " +
			"and the second would read a transcript the first had not finished writing.",
		Boundaries: []string{"runs.admission", "runsegment.opening"},
	}, {
		Key: "terminal_run_explains_how_it_ended",
		Why: "A Run row that claims a terminal state without the outcome — and, when " +
			"that outcome is a failure, the failure itself — cannot answer why the run " +
			"ended, and no later write will supply it.",
		Boundaries: []string{"runsegment.event", "runs.recovery", "sessions.import"},
	}, {
		Key: "run_capabilities_are_immutable",
		Why: "A Run may exercise only the optional behavior enabled at admission. If " +
			"a later segment could restate it, continuation and subscription checks would " +
			"no longer describe the same Run.",
		Boundaries: []string{"runs.admission"},
	}, {
		Key: "parked_tree_has_exactly_one_open_interrupt_set",
		Why: "A Run tree parked without one complete pending set cannot be resumed " +
			"atomically; clients would observe only part of a barrier that can never move.",
		Boundaries: []string{"runsegment.event", "runs.recovery"},
	}, {
		Key: "parked_continuation_matches_run_facts",
		Why: "A continuation is a hand-off of the admitted Run, not a second author. " +
			"If its model, cumulative accounting, limits, lineage, creation time, goal " +
			"lease or capabilities differ, resume or teardown would rewrite history.",
		Boundaries: []string{
			"runsegment.opening",
			"runsegment.event",
			"runsegment.waiting_subtree_cancel",
			"runs.recovery",
			"sessions.parked_terminal",
		},
	}, {
		Key: "dropped_run_leaves_nothing_behind",
		Why: "A dropped Run's items, interrupts, checkpoints and admission slot must " +
			"go with it, or the session keeps an invisible run holding its only slot.",
		Boundaries: []string{"sessions.rollback", "sessions.delete"},
	}, {
		Key: "imported_session_keeps_its_identity",
		Why: "Import is restore, not copy: an artifact must come back under the id it " +
			"was exported with, or its runs and items reference a session that is gone.",
		Boundaries: []string{"sessions.import"},
	}, {
		Key: "goal_never_outlives_its_session",
		Why: "A goal is session-owned. One that survives its session's deletion or " +
			"rollback would keep driving runs toward an objective nobody can see or stop.",
		Boundaries: []string{"goals.lifecycle", "sessions.delete", "sessions.rollback"},
	}}
}
