package runs

import (
	"context"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
)

// runPublications owns the post-commit publication boundary: authoritative
// opening/event/barrier writes, live event timestamps and workspace nudges, and
// the read-model invalidations emitted only after those writes succeed.
type runPublications struct {
	openings      OpeningCommitter
	events        EventCommitter
	barriers      TreeBarrierCommitter
	workspace     WorkspaceChangeNotifier
	invalidations invalidation.Publish
	changes       sessionRunChanges
	now           func() time.Time
}

func newRunPublications(
	projection ProjectionPorts,
	invalidations invalidation.Publish,
	now func() time.Time,
) runPublications {
	return runPublications{
		openings:      projection.Openings,
		events:        projection.Events,
		barriers:      projection.Barriers,
		workspace:     projection.Workspace,
		invalidations: invalidations,
		now:           now,
	}
}

func (r *runPublications) nowUTC() time.Time { return r.now().UTC() }

func (r *runPublications) event(runID, segmentID string, reduced reduction) Event {
	return Event{
		RunID: runID, SegmentID: segmentID,
		Timestamp: r.nowUTC(), Payload: reduced.Event,
	}
}

func (r *runPublications) commitOpening(ctx context.Context, opening OpeningCommit) error {
	return r.openings.CommitOpening(ctx, opening)
}

func (r *runPublications) commitEvent(ctx context.Context, commit EventCommit) error {
	return r.events.CommitEvent(ctx, commit)
}

func (r *runPublications) commitTreeBarrier(
	ctx context.Context,
	barrier TreeBarrierCommit,
) error {
	return r.barriers.CommitTreeBarrier(ctx, barrier)
}

func (r *runPublications) nudge(cwd string, paths []string) {
	r.workspace.Nudge(cwd, paths)
}

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
func (r *runPublications) publishRunMoved(sessionID, runID string) {
	r.changes.notify(sessionID)
	r.invalidations.Notify(
		invalidation.InSession(invalidation.Runs, sessionID, runID),
		invalidation.InSession(invalidation.Sessions, sessionID),
	)
}

// publishWaitingMoved reports a transition that also opened, answered or dropped
// the session's open-interrupt set — a park, a resume, or a canceled park.
func (r *runPublications) publishWaitingMoved(sessionID, runID string) {
	r.changes.notify(sessionID)
	r.invalidations.Notify(
		invalidation.InSession(invalidation.Runs, sessionID, runID),
		invalidation.InSession(invalidation.Interrupts, sessionID, runID),
		invalidation.InSession(invalidation.Sessions, sessionID),
	)
}

// publishWaitingSubtreeCanceled reports the complete read set invalidated by a
// parked child cancellation. affectedRunIDs includes the canceled subtree, the
// parent whose spawning Item was settled, and every Run resumed when the final
// external boundary disappeared. The interrupt notice remains root-addressed
// because one Pending aggregate owns the whole barrier.
func (r *runPublications) publishWaitingSubtreeCanceled(
	sessionID string,
	rootRunID string,
	affectedRunIDs []string,
) {
	r.invalidations.Notify(
		invalidation.InSession(invalidation.Runs, sessionID, affectedRunIDs...),
		invalidation.InSession(invalidation.Interrupts, sessionID, rootRunID),
		invalidation.InSession(invalidation.Sessions, sessionID),
	)
}

// publishGoalMoved reports a run terminal whose commit also charged the session's
// goal. The accounting rides the run's transaction, so nothing in the goal use case
// sees this write — without a notice here, a client would watch a goal spend its
// budget in silence.
func (r *runPublications) publishGoalMoved(sessionID string) {
	r.invalidations.Notify(invalidation.InSession(invalidation.Goals, sessionID))
}
