package runs

import (
	"context"
	"iter"

	"github.com/Tangerg/lynx/app/runtime/internal/application/taskgroup"
)

// segmentLifecycle owns every process-local resource whose lifetime is one
// accepted Segment: task admission/join, executor observation and release,
// replay journals, live addressability, and post-boundary finalization. Durable
// Run state remains in the application commit ports.
type segmentLifecycle struct {
	observations ExecutionObserver
	releases     ExecutionReleaser
	finalizer    SegmentFinalizer
	retention    Retention
	epoch        string
	tasks        taskgroup.Group
	registry     registry
}

func newSegmentLifecycle(
	observations ExecutionObserver,
	releases ExecutionReleaser,
	finalizer SegmentFinalizer,
	retention Retention,
) segmentLifecycle {
	return segmentLifecycle{
		observations: observations,
		releases:     releases,
		finalizer:    finalizer,
		retention:    retention,
		epoch:        newReplayEpoch(),
	}
}

func (lifecycle *segmentLifecycle) replayRetention() Retention { return lifecycle.retention }

func (lifecycle *segmentLifecycle) attach(ctx context.Context) (context.Context, func(), bool) {
	return lifecycle.tasks.Attach(ctx)
}

func (lifecycle *segmentLifecycle) observe(
	ctx context.Context,
	ref ExecutorRef,
) (iter.Seq[ExecutorEvent], error) {
	return lifecycle.observations.Observe(ctx, ref)
}

func (lifecycle *segmentLifecycle) newJournal(runID, segmentID string) *journal {
	return newJournal(streamScope{
		Epoch: lifecycle.epoch, RunID: runID, SegmentID: segmentID,
	}, lifecycle.retention)
}

func (lifecycle *segmentLifecycle) open(record Record, owner *runTreeOwner) {
	lifecycle.registry.Open(record, owner)
}

func (lifecycle *segmentLifecycle) lookup(runID string) (liveSegment, bool) {
	return lifecycle.registry.Get(runID)
}

func (lifecycle *segmentLifecycle) markCancel(runID, reason string) (liveSegment, bool) {
	return lifecycle.registry.MarkCancel(runID, reason)
}

func (lifecycle *segmentLifecycle) remove(runID, segmentID string) (liveSegment, bool) {
	return lifecycle.registry.RemoveSegment(runID, segmentID)
}

func (lifecycle *segmentLifecycle) release(ctx context.Context, ref ExecutorRef) error {
	return lifecycle.releases.Release(ctx, ref)
}

func (lifecycle *segmentLifecycle) finish(ctx context.Context, fin Finish) error {
	return lifecycle.finalizer.Finish(ctx, fin)
}

func (lifecycle *segmentLifecycle) beginShutdown() { lifecycle.tasks.Cancel() }

func (lifecycle *segmentLifecycle) awaitShutdown(ctx context.Context) error {
	return lifecycle.tasks.Wait(ctx)
}
