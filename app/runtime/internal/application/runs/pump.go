package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
)

// pump starts the single goroutine that owns every mutable route and reducer in
// this Segment. Event handling and cleanup are methods on one concrete object;
// no state is shared with another worker.
func (c *Coordinator) pump(
	ctx context.Context,
	ownerCtx context.Context,
	spec segmentSpec,
	executorEvents iter.Seq[ExecutorEvent],
	owner *runTreeOwner,
	routes *executorRoutes,
) {
	pump := &segmentPump{
		coordinator: c,
		ctx:         ctx,
		ownerCtx:    ownerCtx,
		spec:        spec,
		events:      executorEvents,
		owner:       owner,
		routes:      routes,
	}
	pump.publisher = treePublisher{publications: &c.publications, rootSpec: spec, owner: owner}
	pump.run()
}

// segmentPump serializes executor events into durable Run-tree projections. A
// successful terminal or park is committed before publication; a projection
// failure aborts execution rather than exposing an event without durable facts.
type segmentPump struct {
	coordinator *Coordinator
	ctx         context.Context
	ownerCtx    context.Context
	spec        segmentSpec
	events      iter.Seq[ExecutorEvent]
	owner       *runTreeOwner
	routes      *executorRoutes
	publisher   treePublisher

	rootFinished   bool
	rootParked     bool
	abortExecution bool

	pendingToolCommits map[toolCommitKey]ExecutionFactCommit
	childStarts        map[string]*managedChildStart
}

type managedChildStart struct {
	prepared *preparedChildOpening
	outcome  ChildRunStartOutcome
}

type toolCommitKey struct {
	runID  string
	callID string
}

type authoritativeFactResult struct {
	runID              string
	deferred           bool
	settledToolCallIDs []string
}

func (s *segmentPump) run() {
	defer close(s.owner.done)
	defer s.finish()
	for event := range s.events {
		if !s.processEvent(event) {
			return
		}
	}
}

func (s *segmentPump) processEvent(event ExecutorEvent) bool {
	if request, reserving := event.Payload.(ChildRunReservationRequest); reserving {
		return s.handleChildRunReservation(event, request)
	}
	if request, concluding := event.Payload.(ChildRunStartOutcomeRequest); concluding {
		return s.handleChildRunStartOutcome(event, request)
	}
	if err := event.Validate(); err != nil {
		s.fail(err)
		return false
	}
	if commit, authoritative := event.Payload.(ExecutionFactCommit); authoritative {
		if err := commit.validate(); err != nil {
			commit.Complete(err)
			s.fail(err)
			return false
		}
		result, err := s.handleAuthoritativeFact(event.Member, commit.Fact)
		s.completeAuthoritativeFact(commit, result, err)
		// A rejected authoritative write is reported synchronously to the
		// executor. It then produces either a definite failed result or
		// an unknown settlement; stopping this pump here would race that decision
		// and tear down the only source able to report it.
		return true
	}
	if unknown, detected := event.Payload.(UnknownEffectsDetected); detected {
		if err := s.handleUnknownEffects(event.Member, unknown); err != nil {
			s.fail(err)
		}
		return false
	}
	if barrier, interrupted := event.Payload.(TreeInterrupted); interrupted {
		s.handleTreeBarrier(event, barrier)
		return false
	}
	executionFact, ok := event.Payload.(ExecutionFact)
	if !ok {
		s.fail(fmt.Errorf("runs: unsupported executor payload %T", event.Payload))
		return false
	}
	keep, err := s.handleExecutionFact(event.Member, executionFact)
	if err != nil {
		s.fail(err)
		return false
	}
	return keep
}

func (s *segmentPump) handleChildRunReservation(
	event ExecutorEvent,
	request ChildRunReservationRequest,
) bool {
	if err := event.Validate(); err != nil {
		if request.claim() {
			_ = request.complete(ChildRunBinding{}, err)
		}
		s.fail(err)
		return false
	}
	if err := request.validate(); err != nil {
		s.fail(err)
		return false
	}
	if !request.claim() {
		return true
	}
	if existing := s.childStarts[event.Member.MemberID]; existing != nil {
		if existing.prepared.member != event.Member ||
			!existing.prepared.reservation.StartedAt.Equal(request.StartedAt.UTC()) {
			err := fmt.Errorf("runs: child member %q repeated a different start reservation", event.Member.MemberID)
			_ = request.complete(ChildRunBinding{}, err)
			return true
		}
		_ = request.complete(existing.prepared.reservation.Binding, nil)
		return true
	}
	prepared, err := s.coordinator.prepareChildOpening(
		s.spec, s.owner, s.routes, event.Member, request.StartedAt,
	)
	if err == nil {
		err = s.coordinator.childStarts.ReserveChildRunStart(s.ownerCtx, prepared.reservation)
	}
	if err != nil {
		if prepared != nil {
			prepared.releaseBinding(s.owner)
		}
		_ = request.complete(ChildRunBinding{}, err)
		return true
	}
	if s.childStarts == nil {
		s.childStarts = make(map[string]*managedChildStart)
	}
	s.childStarts[event.Member.MemberID] = &managedChildStart{prepared: prepared}
	if err := request.complete(prepared.reservation.Binding, nil); err != nil {
		s.abortPreparedChildStart(prepared)
		delete(s.childStarts, event.Member.MemberID)
		s.fail(err)
		return false
	}
	return true
}

func (s *segmentPump) handleChildRunStartOutcome(
	event ExecutorEvent,
	request ChildRunStartOutcomeRequest,
) bool {
	if err := event.Validate(); err != nil {
		if request.claim() {
			_ = request.complete(err)
		}
		s.fail(err)
		return false
	}
	if err := request.validate(); err != nil {
		s.fail(err)
		return false
	}
	if !request.claim() {
		return true
	}
	managed := s.childStarts[event.Member.MemberID]
	if managed == nil || managed.prepared.member != event.Member ||
		managed.prepared.reservation.Binding != request.Binding {
		err := fmt.Errorf("runs: child member %q has no matching start reservation", event.Member.MemberID)
		_ = request.complete(err)
		return true
	}
	if managed.outcome.Valid() {
		if managed.outcome != request.Outcome {
			err := fmt.Errorf("runs: child member %q repeated a contradictory start outcome", event.Member.MemberID)
			_ = request.complete(err)
			return true
		}
		_ = request.complete(nil)
		return true
	}
	prepared := managed.prepared
	var err error
	keep := true
	switch request.Outcome {
	case ChildRunStarted:
		err = s.coordinator.childStarts.CommitStartedChildRun(
			s.ownerCtx, prepared.reservation, prepared.opening,
		)
		if err == nil {
			managed.outcome = request.Outcome
			s.coordinator.activatePreparedChild(s.spec, s.routes, prepared)
			publication, publishErr := s.publisher.publish(s.ownerCtx, prepared.route, prepared.batch)
			if publishErr != nil || publication.finished || publication.parked {
				if publishErr == nil {
					publishErr = fmt.Errorf("runs: child member %q start unexpectedly reached a boundary", event.Member.MemberID)
				}
				// The durable child Run now exists. Rejecting the executor's started
				// outcome would create a public Run without its executor member, so acknowledge
				// the conclusive start and fail this projection pump instead.
				s.fail(publishErr)
				keep = false
			}
		}
		if err != nil {
			// The executor will not publish a child when this receipt fails. Consume
			// the invisible reservation as aborted; a failed cleanup remains hidden
			// and is reconciled at startup rather than becoming a ghost Run.
			cleanupErr := s.coordinator.childStarts.AbortChildRunStart(
				context.WithoutCancel(s.ownerCtx), prepared.reservation,
			)
			err = errors.Join(err, cleanupErr)
			prepared.releaseBinding(s.owner)
		}
	case ChildRunStartAborted:
		err = s.coordinator.childStarts.AbortChildRunStart(s.ownerCtx, prepared.reservation)
		if err == nil {
			managed.outcome = request.Outcome
			prepared.releaseBinding(s.owner)
		}
	default:
		err = errors.New("runs: invalid child Run start outcome")
	}
	if completionErr := request.complete(err); completionErr != nil {
		s.fail(errors.Join(err, completionErr))
		return false
	}
	return keep
}

func (s *segmentPump) abortPreparedChildStart(prepared *preparedChildOpening) {
	if prepared == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	if s.coordinator.childStarts != nil {
		recordRunCleanupError(ctx, s.coordinator.childStarts.AbortChildRunStart(ctx, prepared.reservation))
	}
	prepared.releaseBinding(s.owner)
}

func (s *segmentPump) handleAuthoritativeFact(
	member ExecutorMember,
	fact ExecutionFact,
) (authoritativeFactResult, error) {
	route, err := s.routes.resolve(member)
	if err != nil {
		return authoritativeFactResult{}, err
	}
	result := authoritativeFactResult{runID: route.runID}
	if route.member.MemberID != "" {
		if bindExecutorMemberErr := s.owner.bindExecutorMember(route.runID, route.member.MemberID); bindExecutorMemberErr != nil {
			return result, bindExecutorMemberErr
		}
	}
	if route.reducer == nil {
		return result, fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID)
	}
	fact = s.classifyChildCancellationFact(route, fact)
	speculative := route.reducer.clone()
	if speculative == nil {
		return result, fmt.Errorf("runs: admitted child run %q has no cloneable reducer", route.runID)
	}
	batch, err := speculative.reduce(fact)
	if err != nil {
		return result, err
	}
	result.settledToolCallIDs = slices.Clone(batch.settledToolCallIDs)
	if toolEnd, endingTool := fact.(ToolCallFinished); endingTool && len(batch.settledToolCallIDs) == 0 {
		// An out-of-order concurrent result is valid speculative reducer state but
		// has no canonical durable prefix yet. Keep its receipt pending until an
		// earlier result can commit the whole prefix atomically.
		if ref := speculative.tools[toolEnd.CallID]; ref != nil && ref.end != nil {
			route.reducer = speculative
			result.deferred = true
			return result, nil
		}
	}
	publication, err := s.publisher.publishAuthoritativeAtomically(
		s.ownerCtx,
		route,
		batch,
	)
	if err != nil {
		// A failed canonical Tool batch must also discard any later results that
		// were buffered speculatively in the live reducer. RunLost synthesis may
		// then close their starts as incomplete, never as persisted successes.
		route.reducer.forgetToolEnds(batch.settledToolCallIDs)
		return result, err
	}
	if publication.finished || publication.parked {
		return result, errors.New("runs: authoritative model/tool fact crossed a segment boundary")
	}
	route.reducer = speculative
	return result, nil
}

func (s *segmentPump) completeAuthoritativeFact(
	current ExecutionFactCommit,
	result authoritativeFactResult,
	err error,
) {
	toolEnd, endingTool := current.Fact.(ToolCallFinished)
	if !endingTool {
		current.Complete(err)
		return
	}
	currentKey := toolCommitKey{runID: result.runID, callID: toolEnd.CallID}
	if result.deferred && err == nil {
		if s.pendingToolCommits == nil {
			s.pendingToolCommits = make(map[toolCommitKey]ExecutionFactCommit)
		}
		if _, duplicate := s.pendingToolCommits[currentKey]; duplicate {
			current.Complete(fmt.Errorf("runs: Tool call %q already has a pending authoritative commit", toolEnd.CallID))
			return
		}
		s.pendingToolCommits[currentKey] = current
		return
	}

	currentCompleted := false
	for _, callID := range result.settledToolCallIDs {
		key := toolCommitKey{runID: result.runID, callID: callID}
		if pending, ok := s.pendingToolCommits[key]; ok {
			delete(s.pendingToolCommits, key)
			pending.Complete(err)
			continue
		}
		if key == currentKey {
			current.Complete(err)
			currentCompleted = true
		}
	}
	if !currentCompleted {
		current.Complete(err)
	}
}

func (s *segmentPump) handleUnknownEffects(
	member ExecutorMember,
	unknown UnknownEffectsDetected,
) error {
	if err := unknown.validate(); err != nil {
		return err
	}
	route, err := s.routes.resolve(member)
	if err != nil {
		return err
	}
	if route != s.routes.root {
		return errors.New("runs: root-only execution reported unknown Effects for a child member")
	}
	if activeChildren := s.routes.unfinishedCount() - 1; activeChildren > 0 {
		return fmt.Errorf("runs: unknown Effects detected with %d active child Runs", activeChildren)
	}
	failure := run.Failure{
		Kind:   run.FailureLost,
		Detail: "an external operation completed without a provable durable result",
	}
	batch, err := route.reducer.reduce(SegmentEnded{
		Reason: run.OutcomeLost, Failure: &failure,
		Duration: route.activeDuration(s.coordinator.publications.nowUTC()),
	})
	if err != nil {
		return fmt.Errorf("runs: reduce unknown Effect loss: %w", err)
	}
	retry := time.NewTicker(100 * time.Millisecond)
	defer retry.Stop()
	for {
		publication, publishErr := s.publisher.publishTerminalAtomically(s.ownerCtx, route, batch)
		if publishErr == nil {
			route.segmentFinished = publication.finished
			s.rootFinished = publication.finished
			s.rootParked = false
			return nil
		}
		trace.SpanFromContext(s.ctx).RecordError(fmt.Errorf("runs: retry unknown Effect loss: %w", publishErr))
		select {
		case <-retry.C:
		case <-s.ownerCtx.Done():
			return errors.Join(publishErr, s.ownerCtx.Err())
		}
	}
}

func (s *segmentPump) handleTreeBarrier(event ExecutorEvent, barrier TreeInterrupted) {
	root, err := s.routes.resolve(event.Member)
	if err != nil {
		s.fail(err)
		return
	}
	if root != s.routes.root {
		s.fail(errors.New("runs: tree interrupt must be emitted by the root executor member"))
		return
	}
	publication, err := s.publisher.publishTreeBarrier(
		s.ownerCtx,
		s.routes,
		barrier,
		s.coordinator.publications.nowUTC(),
	)
	if err != nil {
		s.fail(err)
		return
	}
	if publication.published {
		s.rootFinished = publication.finished
		s.rootParked = publication.parked
	}
}

func (s *segmentPump) handleExecutionFact(member ExecutorMember, executionFact ExecutionFact) (bool, error) {
	route, err := s.routes.resolve(member)
	if err != nil {
		return false, err
	}
	if route.member.MemberID != "" {
		if bindExecutorMemberErr := s.owner.bindExecutorMember(route.runID, route.member.MemberID); bindExecutorMemberErr != nil {
			return false, bindExecutorMemberErr
		}
	}
	if route.reducer == nil {
		return false, fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID)
	}
	if _, interrupted := executionFact.(SegmentInterrupted); interrupted {
		return false, errors.New("runs: executor emitted a per-Run interrupt instead of a tree barrier")
	}
	executionFact = s.classifyChildCancellationFact(route, executionFact)
	if route == s.routes.root && engineEventEndsSegment(executionFact) {
		if activeChildren := s.routes.unfinishedCount() - 1; activeChildren > 0 {
			return false, fmt.Errorf(
				"runs: root run %q reached a segment boundary with %d active child runs",
				route.runID,
				activeChildren,
			)
		}
	}
	projecting := route.reducer
	terminalFact := engineEventEndsSegment(executionFact)
	if terminalFact {
		projecting = route.reducer.clone()
		if projecting == nil {
			return false, fmt.Errorf("runs: run %q has no cloneable terminal reducer", route.runID)
		}
	}
	reductions, err := projecting.reduce(executionFact)
	if err != nil {
		return false, err
	}
	var publication reductionPublication
	if terminalFact {
		publication, err = s.publisher.publishTerminalAtomically(s.ownerCtx, route, reductions)
	} else {
		publication, err = s.publisher.publish(s.ownerCtx, route, reductions)
	}
	if err != nil {
		return false, err
	}
	if terminalFact {
		route.reducer = projecting
	}
	if !publication.published {
		return false, nil
	}
	route.segmentFinished = publication.finished
	if route != s.routes.root {
		return true, nil
	}
	s.rootFinished = s.rootFinished || publication.finished
	s.rootParked = s.rootParked || publication.parked
	// A committed root boundary is the last event this Segment can durably
	// support. Leave a park alive for resume and never consume buffered events
	// after a terminal transition.
	return !s.rootParked && !s.rootFinished, nil
}

func (s *segmentPump) classifyChildCancellationFact(
	route *executorRoute,
	fact ExecutionFact,
) ExecutionFact {
	toolEnd, endingTool := fact.(ToolCallFinished)
	if !endingTool || route == nil || route.reducer == nil {
		return fact
	}
	itemID, open := route.reducer.openToolItemID(toolEnd.CallID)
	if !open {
		return fact
	}
	return s.owner.classifyChildCancellationTool(route.runID, itemID, toolEnd)
}

func (s *segmentPump) fail(err error) {
	s.abortExecution = true
	if s.ctx.Err() == nil && s.ownerCtx.Err() == nil {
		trace.SpanFromContext(s.ctx).RecordError(err)
		s.routes.abortUnfinished()
	}
}

func (s *segmentPump) finish() {
	s.failPendingToolCommits(errors.New("runs: execution ended before concurrent Tool results formed a durable prefix"))
	for memberID, managed := range s.childStarts {
		if !managed.outcome.Valid() {
			s.abortPreparedChildStart(managed.prepared)
		}
		delete(s.childStarts, memberID)
	}
	if !s.rootFinished {
		s.synthesizeUnfinished()
	}
	// Every non-waiting boundary releases the executor tree exactly once. The
	// product outcome is already committed (or will be synthesized immediately
	// above); Release is resource ownership, not a second cancellation decision.
	if !s.rootParked {
		s.tearDownExecutor()
	}
	s.finishBoundary()
}

func (s *segmentPump) failPendingToolCommits(err error) {
	if len(s.pendingToolCommits) == 0 {
		return
	}
	byRun := make(map[string][]string)
	for key, commit := range s.pendingToolCommits {
		byRun[key.runID] = append(byRun[key.runID], key.callID)
		commit.Complete(err)
		delete(s.pendingToolCommits, key)
	}
	for runID, callIDs := range byRun {
		if route := s.routes.byRunID[runID]; route != nil && route.reducer != nil {
			route.reducer.forgetToolEnds(callIDs)
		}
	}
}

// synthesizeUnfinished establishes durable terminal boundaries before executor
// teardown. Children close in canonical postorder; the root closes only if
// every child did, so persistence never advertises a closed tree with a live
// descendant row.
func (s *segmentPump) synthesizeUnfinished() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	ordered, err := s.routes.unfinishedInPostorder()
	childrenClosed := err == nil
	if err != nil {
		s.fail(fmt.Errorf("runs: order unfinished tree for terminal synthesis: %w", err))
	}
	for _, route := range ordered {
		if route != s.routes.root {
			childrenClosed = s.synthesizeRoute(ctx, route) && childrenClosed
		}
	}
	if childrenClosed && !s.routes.root.segmentFinished {
		s.rootFinished = s.synthesizeRoute(ctx, s.routes.root)
		s.rootParked = false
	}
}

func (s *segmentPump) synthesizeRoute(ctx context.Context, route *executorRoute) bool {
	reductions, err := route.reducer.synthesizeTerminal()
	if err != nil {
		s.fail(err)
		return false
	}
	publication, err := s.publisher.publishTerminalAtomically(ctx, route, reductions)
	if err != nil {
		s.fail(err)
		return false
	}
	route.segmentFinished = publication.finished
	if !publication.finished || publication.parked {
		s.fail(fmt.Errorf(
			"runs: synthesized terminal for run %q produced finished=%t parked=%t",
			route.runID,
			publication.finished,
			publication.parked,
		))
		return false
	}
	return true
}

func (s *segmentPump) tearDownExecutor() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
	defer cancel()
	if err := s.coordinator.segments.release(ctx, s.spec.executorRef()); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		s.owner.completionErr = fmt.Errorf("runs: tear down executor %q: %w", s.spec.ExecutorID, err)
		recordRunCleanupError(ctx, s.owner.completionErr)
	}
}

func (s *segmentPump) finishBoundary() {
	releaseMaintenance, maintenanceHeld := s.coordinator.admission.BeginMaintenance(s.spec.RunID)
	entry, tracked := s.coordinator.segments.lookup(s.spec.RunID)
	if tracked && !s.rootParked && entry.owner != nil {
		entry.owner.stop()
	}
	if s.rootFinished {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(s.ownerCtx), runCleanupTimeout)
		if err := s.coordinator.segments.finish(ctx, Finish{
			SessionID:       s.spec.SessionID,
			RunID:           s.spec.RunID,
			CWD:             s.spec.CWD,
			Parked:          s.rootParked,
			OpeningUserText: s.spec.OpeningUserText,
		}); err != nil {
			recordRunCleanupError(ctx, err)
		}
		cancel()
	}
	if maintenanceHeld {
		releaseMaintenance()
	}
	// Closing the journal is the externally observable completion boundary. The
	// synchronous maintenance fence and admission claim must be gone first.
	s.owner.hub.close()
	s.coordinator.segments.remove(s.spec.RunID, s.spec.SegmentID)
}

func engineEventEndsSegment(event ExecutionFact) bool {
	switch event.(type) {
	case SegmentEnded:
		return true
	default:
		return false
	}
}

func recordRunCleanupError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}
