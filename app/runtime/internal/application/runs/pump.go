package runs

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/execution/interrupts"
)

// pump is the run segment goroutine: Start has already atomically committed and
// published the reducer's opening events; the pump reduces each executor
// event and — on a terminal or a drained stream — tears the run down. It commits
// each event's persisted projections before it publishes (§7.2): the projections
// plus the run-state transition they imply (park → interrupted, terminal →
// terminalized; §8.3) land in one transaction BEFORE the event reaches
// subscribers. A client that acts on an event (reads the transcript after a
// terminal, resumes the instant it sees an interrupt) therefore never observes
// state the store does not yet hold, and a terminal frees the persisted admission
// slot before the in-memory one. A projection commit failure aborts the turn
// rather than publishing an event the persisted record cannot back. The interrupt
// commit additionally linearizes against cancel (a cancel that wins the race
// skips the commit). Non-authoritative previews publish directly. A parked run
// leaves its live turn alive for resume; a true terminal cancels it.
func (c *Coordinator) pump(
	ctx context.Context,
	ownerCtx context.Context,
	spec segmentSpec,
	inner iter.Seq[ExecutorEvent],
	live *handle,
	routes *executorRoutes,
) {
	hub := live.hub
	publisher := treePublisher{coordinator: c, rootSpec: spec, live: live}
	rootFinished := false
	rootParked := false
	abortTurn := false
	commitCtx := ownerCtx
	defer close(live.done)
	fail := func(err error) {
		abortTurn = true
		if ctx.Err() == nil && ownerCtx.Err() == nil {
			trace.SpanFromContext(ctx).RecordError(err)
			routes.abortUnfinished()
		}
	}

	defer func() {
		// Shutdown cancels ownerCtx before joining this pump. Terminal synthesis is
		// a persistence cleanup boundary, so it must outlive that signal while remaining
		// bounded; otherwise graceful shutdown itself leaves a Running
		// transcript/admission row for boot recovery to repair.
		if !rootFinished {
			// The stream ended without a segment.finished (canceled mid-flight /
			// drained iterator, or a failed continuation activation) — synthesize
			// every unfinished child from deepest/latest-opened to root, so the
			// durable tree and stream both end child-before-parent. Each reducer
			// decides error-vs-canceled from its state. This runs before executor
			// teardown: a slow or broken CancelTurn must never consume the only
			// budget available for persisted terminal boundaries.
			terminalCtx, cancelTerminal := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			commitCtx = terminalCtx
			childrenClosed := true
			synthesize := func(route *executorRoute) bool {
				reductions, err := route.reducer.synthesizeTerminal()
				if err != nil {
					fail(err)
					return false
				}
				publication, err := publisher.publish(commitCtx, route, reductions)
				if err != nil {
					fail(err)
					return false
				}
				route.segmentFinished = publication.finished
				if !publication.finished || publication.parked {
					fail(fmt.Errorf(
						"runs: synthesized terminal for run %q produced finished=%t parked=%t",
						route.runID,
						publication.finished,
						publication.parked,
					))
					return false
				}
				return true
			}
			for _, route := range routes.unfinishedChildrenInReverseAdmission() {
				childrenClosed = synthesize(route) && childrenClosed
			}
			// A root terminal with an active descendant would advertise a closed
			// tree while a durable child still runs. If any child cleanup could
			// not commit, retain the root's non-terminal row for recovery rather
			// than creating that impossible state.
			if childrenClosed && !routes.root.segmentFinished {
				rootFinished = synthesize(routes.root)
				rootParked = false
			}
			cancelTerminal()
		}
		// A committed park transfers teardown to Cancel's persisted parked-run
		// path. Cancel first removes the open interrupt and terminalizes the Run,
		// then releases the parked executor turn. Tearing it down here merely
		// because requestCancel canceled ctx would reverse that transaction order
		// and leave a persisted interrupt pointing at a missing process on crash.
		if !rootParked && (ctx.Err() != nil || abortTurn) {
			teardownCtx, cancelTeardown := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			if err := c.executor.CancelTurn(teardownCtx, spec.turnRef()); err != nil && !errors.Is(err, ErrTurnNotLive) {
				live.completionErr = fmt.Errorf("runs: tear down turn %q: %w", spec.TurnID, err)
				recordRunCleanupError(teardownCtx, live.completionErr)
			}
			cancelTeardown()
		}
		releaseMaintenance, maintenanceHeld := c.admission.BeginMaintenance(spec.RunID)
		entry, tracked := c.registry.Get(spec.RunID)
		if tracked {
			// A parked run keeps its live turn alive for resume — only cancel +
			// stop on a true terminal. The registry entry remains addressable until
			// the complete join boundary so a repeated Cancel can still wait.
			if !rootParked && entry.handle != nil {
				entry.handle.stop()
			}
		}
		// Maintenance may only observe a boundary the store actually committed.
		// In particular, a failed terminal commit must not create a checkpoint or
		// title that falsely implies the run completed. The session claim above is
		// retained through the synchronous checkpoint fence; title work may detach.
		// A parked commit sets finished too; Effects treats it as non-terminal.
		if rootFinished {
			finishCtx, cancelFinish := context.WithTimeout(context.WithoutCancel(ownerCtx), runCleanupTimeout)
			if err := c.effects.Finish(finishCtx, Finish{
				SessionID:       spec.SessionID,
				RunID:           spec.RunID,
				Cwd:             spec.Cwd,
				Parked:          rootParked,
				OpeningUserText: spec.OpeningUserText,
			}); err != nil {
				recordRunCleanupError(finishCtx, err)
			}
			cancelFinish()
		}
		if maintenanceHeld {
			releaseMaintenance()
		}
		// Journal closure is the externally observable completion boundary. A
		// consumer that drains it may immediately admit the next segment, so the
		// synchronous maintenance fence and its admission claim must be gone first.
		hub.Close()
		c.registry.Remove(spec.RunID)
	}()

	for ev := range inner {
		if request, opening := ev.Payload.(ChildOpeningRequest); opening {
			if err := ev.Validate(); err != nil {
				if request.claim() {
					_ = request.complete(err)
				}
				fail(err)
				return
			}
			if err := request.validate(); err != nil {
				fail(err)
				return
			}
			// Cancellation may win while the request is still buffered. In that
			// case the executor has already rejected its unpublished child and
			// there is no durable transaction to perform.
			if !request.claim() {
				continue
			}
			childRoute, openingBatch, openingErr := c.openChildRun(
				commitCtx,
				spec,
				routes,
				ev.Source,
				request,
				live.CancelReason,
			)
			if openingErr == nil {
				publication, publishErr := publisher.publish(commitCtx, childRoute, openingBatch)
				switch {
				case publishErr != nil:
					openingErr = publishErr
				case publication.finished || publication.parked:
					openingErr = fmt.Errorf(
						"runs: child process %q opening unexpectedly finished its segment",
						ev.Source.ProcessID,
					)
				}
			}
			if confirmationErr := request.complete(openingErr); confirmationErr != nil {
				openingErr = errors.Join(openingErr, confirmationErr)
			}
			if openingErr != nil {
				fail(openingErr)
				return
			}
			continue
		}
		if err := ev.Validate(); err != nil {
			fail(err)
			return
		}
		if barrier, ok := ev.Payload.(TreeInterrupted); ok {
			rootRoute, err := routes.resolve(ev.Source)
			if err != nil {
				fail(err)
				return
			}
			if rootRoute != routes.root {
				fail(errors.New("runs: tree interrupt must be emitted by the root executor source"))
				return
			}
			publication, err := publisher.publishTreeBarrier(
				commitCtx,
				routes,
				barrier,
				c.now().UTC(),
			)
			if err != nil {
				fail(err)
				return
			}
			if !publication.published {
				return
			}
			rootFinished = publication.finished
			rootParked = publication.parked
			return
		}
		route, err := routes.resolve(ev.Source)
		if err != nil {
			fail(err)
			return
		}
		if route.reducer == nil {
			fail(fmt.Errorf("runs: admitted child run %q has no segment reducer", route.runID))
			return
		}
		engineEvent, ok := ev.Payload.(EngineEvent)
		if !ok {
			fail(fmt.Errorf("runs: unsupported executor payload %T", ev.Payload))
			return
		}
		if _, interrupts := engineEvent.(TurnInterrupted); interrupts {
			fail(errors.New("runs: executor emitted a per-Run interrupt instead of a tree barrier"))
			return
		}
		if route == routes.root && engineEventEndsSegment(engineEvent) {
			if active := routes.unfinishedChildrenInReverseAdmission(); len(active) > 0 {
				fail(fmt.Errorf(
					"runs: root run %q reached a segment boundary with %d active child runs",
					route.runID,
					len(active),
				))
				return
			}
		}
		reductions, err := route.reducer.reduce(engineEvent)
		if err != nil {
			fail(err)
			return
		}
		publication, err := publisher.publish(commitCtx, route, reductions)
		if err != nil {
			fail(err)
			return
		}
		if !publication.published {
			return
		}
		route.segmentFinished = publication.finished
		if route != routes.root {
			continue
		}
		rootFinished = rootFinished || publication.finished
		rootParked = rootParked || publication.parked
		if rootParked {
			// Interrupt segment done; leave the turn parked for resume.
			return
		}
		if rootFinished {
			// A committed terminal is the last event this run can durably back: stop
			// before consuming any further buffered event. A cancel that races a turn
			// in the act of parking can emit a TurnInterrupted after the terminal
			// TurnEnd; processing it would try to Suspend an already-terminalized run.
			// The pump owns persisted run-state integrity, so it enforces "nothing after
			// a terminal" here rather than trusting the upstream event ordering.
			return
		}
	}
}

type reductionPublication struct {
	published bool
	finished  bool
	parked    bool
}

func engineEventEndsSegment(event EngineEvent) bool {
	switch event.(type) {
	case TurnEnd:
		return true
	default:
		return false
	}
}

// treePublisher owns the one batch boundary between source-Run reductions and
// their persisted/live projections. Every child keeps its own Run/Segment
// envelope while all events share the root segment's Journal and replay scope.
type treePublisher struct {
	coordinator *Coordinator
	rootSpec    segmentSpec
	live        *handle
}

// publish validates a complete batch before any side effect, then commits every
// persisted fact before appending its event. published=false without an error
// means cancellation won the interrupt-commit race.
func (p treePublisher) publish(
	ctx context.Context,
	route *executorRoute,
	batch reductionBatch,
) (reductionPublication, error) {
	if route == nil {
		return reductionPublication{}, errors.New("runs: publish reductions without an executor route")
	}
	if err := validateReductionBatch(batch); err != nil {
		return reductionPublication{}, err
	}
	if err := validateRouteReductionBatch(route, p.rootSpec.SessionID, batch); err != nil {
		return reductionPublication{}, err
	}
	if batch.parkCommit != nil {
		return reductionPublication{}, fmt.Errorf(
			"runs: run %q produced a per-Run park outside the tree barrier",
			route.runID,
		)
	}
	publication := reductionPublication{published: true}
	goalCharged := false
	for _, reduced := range batch.events {
		// Commit before publish: an event's atomic projection commit (for a terminal,
		// recording the run + terminalizing the run-state) lands before the event
		// is delivered or retained for replay, so a subscriber never observes an
		// event the store doesn't yet back. A commit failure aborts the turn (as
		// the interrupt path does) rather than publishing an unbacked event.
		if reduced.Commit != nil {
			if err := p.coordinator.effects.CommitEvent(ctx, *reduced.Commit); err != nil {
				return reductionPublication{}, fmt.Errorf("runs: commit %T: %w", reduced.Event, err)
			}
			if reduced.Commit.State == StateTerminalize {
				if reduced.Commit.Run == nil {
					return reductionPublication{}, errors.New("runs: terminal commit has no run snapshot")
				}
				if route.lineage.IsRoot() {
					p.live.recordTerminalRun(*reduced.Commit.Run)
				}
			}
			goalCharged = goalCharged || reduced.Commit.GoalTurn != nil
		}
		if reduced.Event.Terminal() {
			publication.finished = true
		}
		p.append(route, reduced)
	}
	if publication.finished {
		p.coordinator.publishRunMoved(p.rootSpec.SessionID, route.runID)
	}
	if goalCharged {
		p.coordinator.publishGoalMoved(p.rootSpec.SessionID)
	}
	return publication, nil
}

type treeBarrierReduction struct {
	route *executorRoute
	batch reductionBatch
}

func (p treePublisher) publishTreeBarrier(
	ctx context.Context,
	routes *executorRoutes,
	barrier TreeInterrupted,
	boundaryAt time.Time,
) (reductionPublication, error) {
	if err := barrier.validate(); err != nil {
		return reductionPublication{}, err
	}
	byProcess := make(map[string][]ProcessSuspension, len(barrier.Suspensions))
	for _, suspension := range barrier.Suspensions {
		route := routes.byProcess[suspension.ProcessID]
		if route == nil || route.segmentFinished {
			return reductionPublication{}, fmt.Errorf(
				"runs: suspension source process %q has no active Run",
				suspension.ProcessID,
			)
		}
		byProcess[suspension.ProcessID] = append(byProcess[suspension.ProcessID], suspension)
	}
	ordered, err := routes.unfinishedInPostorder()
	if err != nil {
		return reductionPublication{}, err
	}

	pending := interrupts.Pending{
		RootRunID:       routes.root.runID,
		SessionID:       p.rootSpec.SessionID,
		TurnID:          p.rootSpec.TurnID,
		ProtocolProfile: routes.root.protocolProfile,
		CreatedAt:       boundaryAt,
	}
	reductions := make([]treeBarrierReduction, 0, len(ordered))
	commits := make([]EventCommit, 0, len(ordered))
	for _, route := range ordered {
		direct := byProcess[route.source.ProcessID]
		var events []RunEvent
		if len(direct) > 0 {
			values := make([]Interrupt, len(direct))
			for index, suspension := range direct {
				values[index] = suspension.Interrupt
			}
			events, err = route.reducer.interrupt(TurnInterrupted{
				Interrupts: values,
				Duration:   route.activeDuration(boundaryAt),
			})
		} else {
			events, err = route.reducer.suspend(route.activeDuration(boundaryAt))
		}
		if err != nil {
			return reductionPublication{}, fmt.Errorf(
				"runs: reduce tree barrier for run %q: %w",
				route.runID,
				err,
			)
		}
		batch, err := route.reducer.project(events)
		if err != nil {
			return reductionPublication{}, fmt.Errorf(
				"runs: project tree barrier for run %q: %w",
				route.runID,
				err,
			)
		}
		if err := validateRouteReductionBatch(route, p.rootSpec.SessionID, batch); err != nil {
			return reductionPublication{}, err
		}
		if batch.parkCommit == nil || batch.parkCommit.Run == nil {
			return reductionPublication{}, fmt.Errorf(
				"runs: tree barrier for run %q produced no suspend commit",
				route.runID,
			)
		}
		run := *batch.parkCommit.Run
		if len(run.Interrupts) != len(direct) {
			return reductionPublication{}, fmt.Errorf(
				"runs: run %q projected %d interrupts from %d suspensions",
				route.runID,
				len(run.Interrupts),
				len(direct),
			)
		}
		pending.Interrupts = append(pending.Interrupts, run.Interrupts...)
		for index, suspension := range direct {
			pending.Suspensions = append(pending.Suspensions, interrupts.SuspensionBinding{
				InterruptItemID: run.Interrupts[index].ItemID,
				ProcessID:       suspension.ProcessID,
				SuspensionID:    suspension.SuspensionID,
			})
		}
		pending.Continuations = append(pending.Continuations, interrupts.Continuation{
			RunID:           route.runID,
			ProcessID:       route.source.ProcessID,
			ParentProcessID: route.source.ParentID,
			SpawnCallID:     route.source.SpawnCallID,
			Lineage:         route.lineage,
			ModelSelection:  route.modelSelection,
			DrainedTools:    slices.Clone(route.reducer.drained),
			RunCreatedAt:    run.CreatedAt,
			Metrics:         run.Metrics,
			Limits:          run.Limits,
		})
		reductions = append(reductions, treeBarrierReduction{route: route, batch: batch})
		commits = append(commits, *batch.parkCommit)
	}
	if err := pending.Validate(); err != nil {
		return reductionPublication{}, fmt.Errorf("runs: build pending interrupt set: %w", err)
	}

	committed, err := p.live.commitInterrupt(ctx, func(interruptCtx context.Context) error {
		if err := p.coordinator.effects.CommitTreeBarrier(interruptCtx, TreeBarrierCommit{
			Pending: pending,
			Runs:    commits,
		}); err != nil {
			return err
		}
		for _, projected := range reductions {
			for _, reduced := range projected.batch.events {
				p.append(projected.route, reduced)
			}
		}
		return nil
	})
	if err != nil {
		return reductionPublication{}, fmt.Errorf("runs: commit tree interrupt barrier: %w", err)
	}
	if !committed {
		return reductionPublication{}, nil
	}
	for _, projected := range reductions {
		projected.route.segmentFinished = true
		p.coordinator.publishWaitingMoved(p.rootSpec.SessionID, projected.route.runID)
	}
	return reductionPublication{published: true, finished: true, parked: true}, nil
}

func (p treePublisher) append(route *executorRoute, reduced reduction) {
	p.live.hub.Append(p.coordinator.event(route.runID, route.segmentID, reduced))
	if reduced.Nudge != nil {
		p.coordinator.effects.Nudge(reduced.Nudge.Cwd, reduced.Nudge.Paths)
	}
	// The projection was written by the tool that reported this event, so the value a
	// notified client reads back is the one the snapshot carries.
	if _, ok := reduced.Event.(StateSnapshot); ok {
		p.coordinator.publishStateMoved(p.rootSpec.SessionID)
	}
}

func recordRunCleanupError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}
