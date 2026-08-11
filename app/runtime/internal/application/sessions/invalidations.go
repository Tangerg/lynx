package sessions

import (
	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// The session lifecycle's invalidations. Each is published from a post-commit
// boundary — for the write-sets, the afterCommit half of [Coordinator.withGoalMutation],
// which runs exactly once and only after the transaction landed.

// publishSessionMoved reports a change to the session row itself: created,
// renamed, relocated, favorited, or branched into existence. Its projections are
// untouched, so nothing else is invalidated.
func (c *Coordinator) publishSessionMoved(sessionID string) {
	c.invalidations.Notify(invalidation.InSession(invalidation.Sessions, sessionID))
}

// publishRunsMoved reports the material transcript projection copied into a new
// session. Items have no independent invalidation topic: runs.changed is the
// contract that tells clients to cold-read both Runs and their Items.
func (c *Coordinator) publishRunsMoved(sessionID string, copied []run.Run) {
	runIDs := make([]string, len(copied))
	for index, value := range copied {
		runIDs[index] = value.ID()
	}
	c.invalidations.Notify(invalidation.Notice{
		Resource: invalidation.Runs, SessionIDs: []string{sessionID}, RunIDs: runIDs,
	})
}

// publishStateMoved reports a committed session-scoped state projection — the value
// a fork seeded, or the one a rollback republished.
func (c *Coordinator) publishStateMoved(sessionIDs ...string) {
	c.invalidations.Notify(invalidation.InSessions(invalidation.PlanState, sessionIDs...))
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
	c.invalidations.Notify(
		invalidation.InSessions(invalidation.Sessions, sessionIDs...),
		invalidation.Notice{Resource: invalidation.Runs, SessionIDs: sessionIDs, RunIDs: runIDs},
		invalidation.InSessions(invalidation.Interrupts, sessionIDs...),
		invalidation.InSessions(invalidation.Goals, sessionIDs...),
		invalidation.InSessions(invalidation.PlanState, sessionIDs...),
	)
}
