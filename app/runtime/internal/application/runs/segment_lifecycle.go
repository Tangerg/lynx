package runs

import (
	"context"
	"iter"

	"github.com/Tangerg/scope/app/runtime/internal/application/taskgroup"
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

func (s *segmentLifecycle) replayRetention() Retention { return s.retention }

func (s *segmentLifecycle) attach(ctx context.Context) (context.Context, func(), bool) {
	return s.tasks.Attach(ctx)
}

func (s *segmentLifecycle) observe(
	ctx context.Context,
	ref ExecutorRef,
) (iter.Seq[ExecutorEvent], error) {
	return s.observations.Observe(ctx, ref)
}

func (s *segmentLifecycle) newJournal(runID, segmentID string) *journal {
	return newJournal(streamScope{
		Epoch: s.epoch, RunID: runID, SegmentID: segmentID,
	}, s.retention)
}

func (s *segmentLifecycle) open(record Record, owner *runTreeOwner) {
	s.registry.Open(record, owner)
}

func (s *segmentLifecycle) lookup(runID string) (liveSegment, bool) {
	return s.registry.Get(runID)
}

func (s *segmentLifecycle) markCancel(runID, reason string) (liveSegment, bool) {
	return s.registry.MarkCancel(runID, reason)
}

func (s *segmentLifecycle) remove(runID, segmentID string) (liveSegment, bool) {
	return s.registry.RemoveSegment(runID, segmentID)
}

func (s *segmentLifecycle) release(ctx context.Context, ref ExecutorRef) error {
	return s.releases.Release(ctx, ref)
}

func (s *segmentLifecycle) finish(ctx context.Context, fin Finish) error {
	return s.finalizer.Finish(ctx, fin)
}

func (s *segmentLifecycle) beginShutdown() { s.tasks.Cancel() }

func (s *segmentLifecycle) awaitShutdown(ctx context.Context) error {
	return s.tasks.Wait(ctx)
}
