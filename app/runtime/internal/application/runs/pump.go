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
	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
)

// pump starts the single goroutine that owns every mutable route and reducer in
// this Segment. Event handling and cleanup are methods on one concrete object;
// no state is shared with another worker.
func (c *Coordinator) pump(
	ctx context.Context,
	ownerCtx context.Context,
	spec segmentSpec,
	inner iter.Seq[ExecutorEvent],
	live *handle,
	routes *executorRoutes,
) {
	pump := &segmentPump{
		coordinator: c,
		ctx:         ctx,
		ownerCtx:    ownerCtx,
		spec:        spec,
		events:      inner,
		live:        live,
		routes:      routes,
	}
	pump.publisher = treePublisher{coordinator: c, rootSpec: spec, live: live}
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
	live        *handle
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

func (p *segmentPump) run() {
	defer close(p.live.done)
	defer p.finish()
	for event := range p.events {
		if !p.handle(event) {
			return
		}
	}
}

func (p *segmentPump) handle(event ExecutorEvent) bool {
	if request, reserving := event.Payload.(ChildRunReservationRequest); reserving {
		return p.handleChildRunReservation(event, request)
	}
	if request, concluding := event.Payload.(ChildRunStartOutcomeRequest); concluding {
		return p.handleChildRunStartOutcome(event, request)
	}
	if err := event.Validate(); err != nil {
		p.fail(err)
		return false
	}
	if commit, authoritative := event.Payload.(ExecutionFactCommit); authoritative {
		if err := commit.validate(); err != nil {
			commit.Complete(err)
			p.fail(err)
			return false
		}
		result, err := p.handleAuthoritativeFact(event.Member, commit.Fact)
		p.completeAuthoritativeFact(commit, result, err)
		// A rejected authoritative write is reported synchronously to the
		// dispatcher. The Process then produces either a definite failed result or
		// an unknown settlement; stopping this pump here would race that decision
		// and tear down the only source able to report it.
		return true
	}
	if unknown, detected := event.Payload.(UnknownEffectsDetected); detected {
		if err := p.handleUnknownEffects(event.Member, unknown); err != nil {
			p.fail(err)
		}
		return false
	}
	if barrier, interrupted := event.Payload.(TreeInterrupted); interrupted {
		p.handleTreeBarrier(event, barrier)
		return false
	}
	executionFact, ok := event.Payload.(ExecutionFact)
	if !ok {
		p.fail(fmt.Errorf("runs: unsupported executor payload %T", event.Payload))
		return false
	}
	keep, err := p.handleExecutionFact(event.Member, executionFact)
	if err != nil {
		p.fail(err)
		return false
	}
	return keep
}

func (p *segmentPump) handleChildRunReservation(
	event ExecutorEvent,
	request ChildRunReservationRequest,
) bool {
	if err := event.Validate(); err != nil {
		if request.claim() {
			_ = request.complete(ChildRunBinding{}, err)
		}
		p.fail(err)
		return false
	}
	if err := request.validate(); err != nil {
		p.fail(err)
		return false
	}
	if !request.claim() {
		return true
	}
	if p.coordinator.childStarts == nil {
		err := errors.New("runs: child Run start persistence is unavailable")
		_ = request.complete(ChildRunBinding{}, err)
		return true
	}
	if existing := p.childStarts[event.Member.MemberID]; existing != nil {
		if existing.prepared.member != event.Member ||
			!existing.prepared.reservation.StartedAt.Equal(request.StartedAt.UTC()) {
			err := fmt.Errorf("runs: child member %q repeated a different start reservation", event.Member.MemberID)
			_ = request.complete(ChildRunBinding{}, err)
			return true
		}
		_ = request.complete(existing.prepared.reservation.Binding, nil)
		return true
	}
	prepared, err := p.coordinator.prepareChildOpening(
		p.spec, p.live, p.routes, event.Member, request.StartedAt,
	)
	if err == nil {
		err = p.coordinator.childStarts.ReserveChildRunStart(p.ownerCtx, prepared.reservation)
	}
	if err != nil {
		if prepared != nil {
			prepared.releaseBinding(p.live)
		}
		_ = request.complete(ChildRunBinding{}, err)
		return true
	}
	if p.childStarts == nil {
		p.childStarts = make(map[string]*managedChildStart)
	}
	p.childStarts[event.Member.MemberID] = &managedChildStart{prepared: prepared}
	if err := request.complete(prepared.reservation.Binding, nil); err != nil {
		p.abortPreparedChildStart(prepared)
		delete(p.childStarts, event.Member.MemberID)
		p.fail(err)
		return false
	}
	return true
}

func (p *segmentPump) handleChildRunStartOutcome(
	event ExecutorEvent,
	request ChildRunStartOutcomeRequest,
) bool {
	if err := event.Validate(); err != nil {
		if request.claim() {
			_ = request.complete(err)
		}
		p.fail(err)
		return false
	}
	if err := request.validate(); err != nil {
		p.fail(err)
		return false
	}
	if !request.claim() {
		return true
	}
	managed := p.childStarts[event.Member.MemberID]
	if managed == nil || managed.prepared.member != event.Member ||
		managed.prepared.reservation.Binding != request.Binding {
		err := fmt.Errorf("runs: child member %q has no matching start reservation", event.Member.MemberID)
		_ = request.complete(err)
		return true
	}
	if managed.outcome.valid() {
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
		err = p.coordinator.childStarts.CommitStartedChildRun(
			p.ownerCtx, prepared.reservation, prepared.opening,
		)
		if err == nil {
			managed.outcome = request.Outcome
			p.coordinator.activatePreparedChild(p.spec, p.routes, prepared)
			publication, publishErr := p.publisher.publish(p.ownerCtx, prepared.route, prepared.batch)
			if publishErr != nil || publication.finished || publication.parked {
				if publishErr == nil {
					publishErr = fmt.Errorf("runs: child member %q start unexpectedly reached a boundary", event.Member.MemberID)
				}
				// The durable child Run now exists. Rejecting the executor's started
				// outcome would create a public Run without a Process, so acknowledge
				// the conclusive start and fail this projection pump instead.
				p.fail(publishErr)
				keep = false
			}
		}
		if err != nil {
			// The executor will not publish a child when this receipt fails. Consume
			// the invisible reservation as aborted; a failed cleanup remains hidden
			// and is reconciled at startup rather than becoming a ghost Run.
			cleanupErr := p.coordinator.childStarts.AbortChildRunStart(
				context.WithoutCancel(p.ownerCtx), prepared.reservation,
			)
			err = errors.Join(err, cleanupErr)
			prepared.releaseBinding(p.live)
		}
	case ChildRunStartAborted:
		err = p.coordinator.childStarts.AbortChildRunStart(p.ownerCtx, prepared.reservation)
		if err == nil {
			managed.outcome = request.Outcome
			prepared.releaseBinding(p.live)
		}
	default:
		err = errors.New("runs: invalid child Run start outcome")
	}
	if completionErr := request.complete(err); completionErr != nil {
		p.fail(errors.Join(err, completionErr))
		return false
	}
	return keep
}

func (p *segmentPump) abortPreparedChildStart(prepared *preparedChildOpening) {
	if prepared == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(p.ownerCtx), runCleanupTimeout)
	defer cancel()
	if p.coordinator.childStarts != nil {
		recordRunCleanupError(ctx, p.coordinator.childStarts.AbortChildRunStart(ctx, prepared.reservation))
	}
	prepared.releaseBinding(p.live)
}

func (p *segmentPump) handleAuthoritativeFact(
	member ExecutorMember,
	fact ExecutionFact,
) (authoritativeFactResult, error) {
	route, err := p.routes.resolve(member)
	if err != nil {
		return authoritativeFactResult{}, err
	}
	result := authoritativeFactResult{runID: route.runID}
	if route.member.MemberID != "" {
		if err := p.live.bindExecutorMember(route.runID, route.member.MemberID); err != nil {
			return result, err
		}
	}
	if route.reducer == nil {
		return result, fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID)
	}
	fact = p.classifyChildCancellationFact(route, fact)
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
	publication, err := p.publisher.publishAuthoritativeAtomically(
		p.ownerCtx,
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

func (p *segmentPump) completeAuthoritativeFact(
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
		if p.pendingToolCommits == nil {
			p.pendingToolCommits = make(map[toolCommitKey]ExecutionFactCommit)
		}
		if _, duplicate := p.pendingToolCommits[currentKey]; duplicate {
			current.Complete(fmt.Errorf("runs: Tool call %q already has a pending authoritative commit", toolEnd.CallID))
			return
		}
		p.pendingToolCommits[currentKey] = current
		return
	}

	currentCompleted := false
	for _, callID := range result.settledToolCallIDs {
		key := toolCommitKey{runID: result.runID, callID: callID}
		if pending, ok := p.pendingToolCommits[key]; ok {
			delete(p.pendingToolCommits, key)
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

func (p *segmentPump) handleUnknownEffects(
	member ExecutorMember,
	unknown UnknownEffectsDetected,
) error {
	if err := unknown.validate(); err != nil {
		return err
	}
	route, err := p.routes.resolve(member)
	if err != nil {
		return err
	}
	if route != p.routes.root {
		return errors.New("runs: root-only execution reported unknown Effects for a child member")
	}
	if activeChildren := p.routes.unfinishedCount() - 1; activeChildren > 0 {
		return fmt.Errorf("runs: unknown Effects detected with %d active child Runs", activeChildren)
	}
	problem := transcript.Problem{
		Kind: transcript.RunLostProblem, Scope: transcript.RunProblem,
		Detail: "an external operation completed without a provable durable result",
	}
	batch, err := route.reducer.reduce(SegmentEnded{
		Reason: run.OutcomeLost, Problem: &problem,
		Duration: route.activeDuration(p.coordinator.now().UTC()),
	})
	if err != nil {
		return fmt.Errorf("runs: reduce unknown Effect loss: %w", err)
	}
	retry := time.NewTicker(100 * time.Millisecond)
	defer retry.Stop()
	for {
		publication, publishErr := p.publisher.publishTerminalAtomically(p.ownerCtx, route, batch)
		if publishErr == nil {
			route.segmentFinished = publication.finished
			p.rootFinished = publication.finished
			p.rootParked = false
			return nil
		}
		trace.SpanFromContext(p.ctx).RecordError(fmt.Errorf("runs: retry unknown Effect loss: %w", publishErr))
		select {
		case <-retry.C:
		case <-p.ownerCtx.Done():
			return errors.Join(publishErr, p.ownerCtx.Err())
		}
	}
}

func (p *segmentPump) handleTreeBarrier(event ExecutorEvent, barrier TreeInterrupted) {
	root, err := p.routes.resolve(event.Member)
	if err != nil {
		p.fail(err)
		return
	}
	if root != p.routes.root {
		p.fail(errors.New("runs: tree interrupt must be emitted by the root executor member"))
		return
	}
	publication, err := p.publisher.publishTreeBarrier(
		p.ownerCtx,
		p.routes,
		barrier,
		p.coordinator.now().UTC(),
	)
	if err != nil {
		p.fail(err)
		return
	}
	if publication.published {
		p.rootFinished = publication.finished
		p.rootParked = publication.parked
	}
}

func (p *segmentPump) handleExecutionFact(member ExecutorMember, executionFact ExecutionFact) (bool, error) {
	route, err := p.routes.resolve(member)
	if err != nil {
		return false, err
	}
	if route.member.MemberID != "" {
		if err := p.live.bindExecutorMember(route.runID, route.member.MemberID); err != nil {
			return false, err
		}
	}
	if route.reducer == nil {
		return false, fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID)
	}
	if _, interrupted := executionFact.(SegmentInterrupted); interrupted {
		return false, errors.New("runs: executor emitted a per-Run interrupt instead of a tree barrier")
	}
	executionFact = p.classifyChildCancellationFact(route, executionFact)
	if route == p.routes.root && engineEventEndsSegment(executionFact) {
		if activeChildren := p.routes.unfinishedCount() - 1; activeChildren > 0 {
			return false, fmt.Errorf(
				"runs: root run %q reached a segment boundary with %d active child runs",
				route.runID,
				activeChildren,
			)
		}
	}
	reductions, err := route.reducer.reduce(executionFact)
	if err != nil {
		return false, err
	}
	publication, err := p.publisher.publish(p.ownerCtx, route, reductions)
	if err != nil {
		return false, err
	}
	if !publication.published {
		return false, nil
	}
	route.segmentFinished = publication.finished
	if route != p.routes.root {
		return true, nil
	}
	p.rootFinished = p.rootFinished || publication.finished
	p.rootParked = p.rootParked || publication.parked
	// A committed root boundary is the last event this Segment can durably
	// support. Leave a park alive for resume and never consume buffered events
	// after a terminal transition.
	return !p.rootParked && !p.rootFinished, nil
}

func (p *segmentPump) classifyChildCancellationFact(
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
	return p.live.classifyChildCancellationTool(route.runID, itemID, toolEnd)
}

func (p *segmentPump) fail(err error) {
	p.abortExecution = true
	if p.ctx.Err() == nil && p.ownerCtx.Err() == nil {
		trace.SpanFromContext(p.ctx).RecordError(err)
		p.routes.abortUnfinished()
	}
}

func (p *segmentPump) finish() {
	p.failPendingToolCommits(errors.New("runs: execution ended before concurrent Tool results formed a durable prefix"))
	for memberID, managed := range p.childStarts {
		if !managed.outcome.valid() {
			p.abortPreparedChildStart(managed.prepared)
		}
		delete(p.childStarts, memberID)
	}
	if !p.rootFinished {
		p.synthesizeUnfinished()
	}
	// Every non-waiting boundary releases the executor tree exactly once. The
	// product outcome is already committed (or will be synthesized immediately
	// above); Release is resource ownership, not a second cancellation decision.
	if !p.rootParked {
		p.tearDownExecutor()
	}
	p.finishBoundary()
}

func (p *segmentPump) failPendingToolCommits(err error) {
	if len(p.pendingToolCommits) == 0 {
		return
	}
	byRun := make(map[string][]string)
	for key, commit := range p.pendingToolCommits {
		byRun[key.runID] = append(byRun[key.runID], key.callID)
		commit.Complete(err)
		delete(p.pendingToolCommits, key)
	}
	for runID, callIDs := range byRun {
		if route := p.routes.byRunID[runID]; route != nil && route.reducer != nil {
			route.reducer.forgetToolEnds(callIDs)
		}
	}
}

// synthesizeUnfinished establishes durable terminal boundaries before executor
// teardown. Children close in canonical postorder; the root closes only if
// every child did, so persistence never advertises a closed tree with a live
// descendant row.
func (p *segmentPump) synthesizeUnfinished() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(p.ownerCtx), runCleanupTimeout)
	defer cancel()
	ordered, err := p.routes.unfinishedInPostorder()
	childrenClosed := err == nil
	if err != nil {
		p.fail(fmt.Errorf("runs: order unfinished tree for terminal synthesis: %w", err))
	}
	for _, route := range ordered {
		if route != p.routes.root {
			childrenClosed = p.synthesizeRoute(ctx, route) && childrenClosed
		}
	}
	if childrenClosed && !p.routes.root.segmentFinished {
		p.rootFinished = p.synthesizeRoute(ctx, p.routes.root)
		p.rootParked = false
	}
}

func (p *segmentPump) synthesizeRoute(ctx context.Context, route *executorRoute) bool {
	reductions, err := route.reducer.synthesizeTerminal()
	if err != nil {
		p.fail(err)
		return false
	}
	publication, err := p.publisher.publish(ctx, route, reductions)
	if err != nil {
		p.fail(err)
		return false
	}
	route.segmentFinished = publication.finished
	if !publication.finished || publication.parked {
		p.fail(fmt.Errorf(
			"runs: synthesized terminal for run %q produced finished=%t parked=%t",
			route.runID,
			publication.finished,
			publication.parked,
		))
		return false
	}
	return true
}

func (p *segmentPump) tearDownExecutor() {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(p.ownerCtx), runCleanupTimeout)
	defer cancel()
	if err := p.coordinator.releases.Release(ctx, p.spec.executorRef()); err != nil && !errors.Is(err, ErrExecutorNotLive) {
		p.live.completionErr = fmt.Errorf("runs: tear down executor %q: %w", p.spec.ExecutorID, err)
		recordRunCleanupError(ctx, p.live.completionErr)
	}
}

func (p *segmentPump) finishBoundary() {
	releaseMaintenance, maintenanceHeld := p.coordinator.admission.BeginMaintenance(p.spec.RunID)
	entry, tracked := p.coordinator.registry.Get(p.spec.RunID)
	if tracked && !p.rootParked && entry.handle != nil {
		entry.handle.stop()
	}
	if p.rootFinished {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(p.ownerCtx), runCleanupTimeout)
		if err := p.coordinator.finalizer.Finish(ctx, Finish{
			SessionID:       p.spec.SessionID,
			RunID:           p.spec.RunID,
			CWD:             p.spec.CWD,
			Parked:          p.rootParked,
			OpeningUserText: p.spec.OpeningUserText,
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
	p.live.hub.close()
	p.coordinator.registry.Remove(p.spec.RunID)
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
