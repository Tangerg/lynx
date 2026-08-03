package sessions

import "github.com/Tangerg/lynx/app/runtime/internal/application/change"

// The session lifecycle's invalidations. Each is published from a post-commit
// boundary — for the write-sets, the afterCommit half of [Coordinator.withGoalMutation],
// which runs exactly once and only after the transaction landed.

// publishSessionMoved reports a change to the session row itself: created,
// renamed, relocated, favorited, or branched into existence. Its projections are
// untouched, so nothing else is invalidated.
func (c *Coordinator) publishSessionMoved(sessionID string) {
	c.changed.Notify(change.InSession(change.Sessions, sessionID))
}

// publishStateMoved reports a committed session-scoped state projection — the value
// a fork seeded, or the one a rollback republished.
func (c *Coordinator) publishStateMoved(sessionIDs ...string) {
	c.changed.Notify(change.InSessions(change.PlanState, sessionIDs...))
}

// publishAggregateMoved reports a write-set that replaced or removed everything a
// session owns: the delete cascade, a history rollback, an import over an existing
// session. It names every projection keyed by the session because that is precisely
// what those transactions touch — the session row, its runs, the interrupts they
// were parked on, its goal, and its Plan. A client told only "sessions
// changed" would keep showing a run list belonging to runs that no longer exist.
//
// runIDs narrows the run signal when the caller knows which runs went (a rollback
// boundary does); empty means every run of these sessions may have moved.
func (c *Coordinator) publishAggregateMoved(sessionIDs []string, runIDs []string) {
	c.changed.Notify(
		change.InSessions(change.Sessions, sessionIDs...),
		change.Notice{Resource: change.Runs, SessionIDs: sessionIDs, RunIDs: runIDs},
		change.InSessions(change.Interrupts, sessionIDs...),
		change.InSessions(change.Goals, sessionIDs...),
		change.InSessions(change.PlanState, sessionIDs...),
	)
}
