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
	if request, opening := event.Payload.(ChildOpeningRequest); opening {
		return p.handleChildOpening(event, request)
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

func (p *segmentPump) handleChildOpening(event ExecutorEvent, request ChildOpeningRequest) bool {
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
	// Cancellation may win while the request is buffered. The executor then
	// rejects its unpublished child, so there is no durable opening to perform.
	if !request.claim() {
		return true
	}
	child, opening, err := p.coordinator.openChildRun(
		p.ownerCtx,
		p.spec,
		p.live,
		p.routes,
		event.Member,
		request,
	)
	if err == nil {
		var publication reductionPublication
		publication, err = p.publisher.publish(p.ownerCtx, child, opening)
		if err == nil && (publication.finished || publication.parked) {
			err = fmt.Errorf(
				"runs: child member %q opening unexpectedly finished its segment",
				event.Member.MemberID,
			)
		}
	}
	binding := ChildRunBinding{}
	if err == nil {
		binding = ChildRunBinding{
			MemberID:    event.Member.MemberID,
			RunID:       child.runID,
			ParentRunID: child.lineage.ParentRunID,
		}
	}
	if confirmationErr := request.complete(binding, err); confirmationErr != nil {
		err = errors.Join(err, confirmationErr)
	}
	if err != nil {
		p.fail(err)
		return false
	}
	return true
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
	if toolEnd, endingTool := executionFact.(ToolCallFinished); endingTool {
		if itemID, open := route.reducer.openToolItemID(toolEnd.CallID); open {
			executionFact = p.live.classifyChildCancellationTool(route.runID, itemID, toolEnd)
		}
	}
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

func (p *segmentPump) fail(err error) {
	p.abortExecution = true
	if p.ctx.Err() == nil && p.ownerCtx.Err() == nil {
		trace.SpanFromContext(p.ctx).RecordError(err)
		p.routes.abortUnfinished()
	}
}

func (p *segmentPump) finish() {
	p.failPendingToolCommits(errors.New("runs: execution ended before concurrent Tool results formed a durable prefix"))
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
	// Closing the Journal is the externally observable completion boundary. The
	// synchronous maintenance fence and admission claim must be gone first.
	p.live.hub.Close()
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
