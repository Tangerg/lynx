package runs

import "github.com/Tangerg/lynx/app/runtime/internal/application/change"

// The run lifecycle's invalidations for clients that are not following this run.
// They are published AFTER the durable transition they describe, from the same
// places that already own it — a client told a run moved and then reading the old
// record would be worse than not being told.
//
// A session's status is derived from its runs, so every run transition moves the
// session too; the notices stay separate because a client folds them into different
// reads (a run list and a session list), and one signal for two reads would make
// every listener refetch both.

// publishRunMoved reports a run whose lifecycle position changed without touching
// what it is waiting on: a run that started, or one that ended.
func (c *Coordinator) publishRunMoved(sessionID, runID string) {
	c.changed.Notify(
		change.InSession(change.Runs, sessionID, runID),
		change.InSession(change.Sessions, sessionID),
	)
}

// publishWaitingMoved reports a transition that also opened, answered or dropped
// the session's open-interrupt set — a park, a resume, or a canceled park.
func (c *Coordinator) publishWaitingMoved(sessionID, runID string) {
	c.changed.Notify(
		change.InSession(change.Runs, sessionID, runID),
		change.InSession(change.Interrupts, sessionID, runID),
		change.InSession(change.Sessions, sessionID),
	)
}

// publishWaitingSubtreeCanceled reports the complete read set invalidated by a
// parked child cancellation. affectedRunIDs includes the canceled subtree, the
// parent whose spawning Item was settled, and every Run resumed when the final
// external boundary disappeared. The interrupt notice remains root-addressed
// because one Pending aggregate owns the whole barrier.
func (c *Coordinator) publishWaitingSubtreeCanceled(
	sessionID string,
	rootRunID string,
	affectedRunIDs []string,
) {
	c.changed.Notify(
		change.InSession(change.Runs, sessionID, affectedRunIDs...),
		change.InSession(change.Interrupts, sessionID, rootRunID),
		change.InSession(change.Sessions, sessionID),
	)
}

// publishGoalMoved reports a run terminal whose commit also charged the session's
// goal. The accounting rides the run's transaction, so nothing in the goal use case
// sees this write — without a notice here, a client would watch a goal spend its
// budget in silence.
func (c *Coordinator) publishGoalMoved(sessionID string) {
	c.changed.Notify(change.InSession(change.Goals, sessionID))
}

// publishStateMoved reports a committed session-scoped state projection. The run's
// own stream carries the snapshot itself (§6.2); this only says "read that key
// again", for the clients the stream does not reach.
func (c *Coordinator) publishStateMoved(sessionID string) {
	c.changed.Notify(change.InSession(change.TodoState, sessionID))
}
