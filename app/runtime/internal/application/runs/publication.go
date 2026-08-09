package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

type reductionPublication struct {
	published bool
	finished  bool
	parked    bool
}

// treePublisher owns the batch boundary between source-Run reductions and
// their persisted and live projections. Every child keeps its own Run/Segment
// envelope while all events share the root Segment's journal and replay scope.
type treePublisher struct {
	coordinator *Coordinator
	rootSpec    segmentSpec
	owner       *runTreeOwner
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
		// Every durable projection lands before its event is delivered or retained
		// for replay. A failed commit aborts execution instead of publishing an
		// event the stores do not yet support.
		if reduced.Commit != nil {
			if reduced.Commit.State == StateTerminalize && route.member.ParentID == "" {
				reduced.Commit.ObsoleteCheckpointRootID = route.member.MemberID
			}
			if err := p.coordinator.events.CommitEvent(ctx, *reduced.Commit); err != nil {
				return reductionPublication{}, fmt.Errorf("runs: commit %T: %w", reduced.Event, err)
			}
			if reduced.Commit.State == StateTerminalize {
				if reduced.Commit.Run == nil {
					return reductionPublication{}, errors.New("runs: terminal commit has no run snapshot")
				}
				p.owner.recordTerminalRun(*reduced.Commit.Run)
			}
			for _, item := range reduced.Commit.Items {
				p.owner.recordChildCancellationItem(route.runID, item)
			}
			goalCharged = goalCharged || reduced.Commit.GoalRun != nil
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

// publishAuthoritativeAtomically commits every durable projection derived from
// one model/tool fact in a single transaction before publishing any live event.
// The caller reduces against a speculative reducer and swaps that state in only
// after this method succeeds, so a failed final/result/usage write leaves the
// previously committed start boundary intact for unknown reconciliation.
func (p treePublisher) publishAuthoritativeAtomically(
	ctx context.Context,
	route *executorRoute,
	batch reductionBatch,
) (reductionPublication, error) {
	if route == nil {
		return reductionPublication{}, errors.New("runs: publish authoritative fact without an executor route")
	}
	if err := validateReductionBatch(batch); err != nil {
		return reductionPublication{}, err
	}
	if err := validateRouteReductionBatch(route, p.rootSpec.SessionID, batch); err != nil {
		return reductionPublication{}, err
	}
	if batch.parkCommit != nil {
		return reductionPublication{}, errors.New("runs: authoritative fact unexpectedly produced a park boundary")
	}
	combined := EventCommit{RunID: route.runID, SessionID: p.rootSpec.SessionID}
	for index, reduced := range batch.events {
		if reduced.Event.Terminal() {
			return reductionPublication{}, fmt.Errorf(
				"runs: authoritative fact unexpectedly produced terminal event[%d]",
				index,
			)
		}
		if reduced.Commit == nil {
			continue
		}
		if reduced.Commit.State != StateUnchanged || reduced.Commit.Run != nil || reduced.Commit.GoalRun != nil {
			return reductionPublication{}, fmt.Errorf(
				"runs: authoritative fact event[%d] carries a lifecycle transition",
				index,
			)
		}
		combined.Items = append(combined.Items, reduced.Commit.Items...)
		combined.ModelInvocations = append(
			combined.ModelInvocations,
			reduced.Commit.ModelInvocations...,
		)
		combined.ToolInvocations = append(
			combined.ToolInvocations,
			reduced.Commit.ToolInvocations...,
		)
		if reduced.Commit.Progress != nil {
			if combined.Progress != nil {
				return reductionPublication{}, errors.New("runs: authoritative fact repeats Run progress")
			}
			progress := *reduced.Commit.Progress
			combined.Progress = &progress
		}
	}
	if !combined.isEmpty() {
		if err := combined.Validate(); err != nil {
			return reductionPublication{}, fmt.Errorf("runs: validate authoritative fact: %w", err)
		}
		if err := p.coordinator.events.CommitEvent(ctx, combined); err != nil {
			return reductionPublication{}, fmt.Errorf("runs: commit authoritative fact: %w", err)
		}
		for _, item := range combined.Items {
			p.owner.recordChildCancellationItem(route.runID, item)
		}
	}
	for _, reduced := range batch.events {
		p.append(route, reduced)
	}
	return reductionPublication{published: true}, nil
}

// publishTerminalAtomically commits every item closure and the terminal Run in
// one EventCommit before publishing any event. Unknown external outcomes use
// this path so a failed transaction leaves the live executor tree blocked and the
// exact immutable batch can be retried without exposing a partial RunLost fact.
func (p treePublisher) publishTerminalAtomically(
	ctx context.Context,
	route *executorRoute,
	batch reductionBatch,
) (reductionPublication, error) {
	if route == nil {
		return reductionPublication{}, errors.New("runs: publish atomic terminal without an executor route")
	}
	if err := validateReductionBatch(batch); err != nil {
		return reductionPublication{}, err
	}
	if err := validateRouteReductionBatch(route, p.rootSpec.SessionID, batch); err != nil {
		return reductionPublication{}, err
	}
	combined := EventCommit{RunID: route.runID, SessionID: p.rootSpec.SessionID}
	terminalEvents := 0
	for _, reduced := range batch.events {
		if reduced.Commit != nil {
			combined.Items = append(combined.Items, reduced.Commit.Items...)
			combined.ModelInvocations = append(
				combined.ModelInvocations,
				reduced.Commit.ModelInvocations...,
			)
			combined.ToolInvocations = append(
				combined.ToolInvocations,
				reduced.Commit.ToolInvocations...,
			)
			if reduced.Commit.Progress != nil {
				if combined.Progress != nil {
					return reductionPublication{}, errors.New("runs: atomic terminal batch repeats Run progress")
				}
				progress := *reduced.Commit.Progress
				combined.Progress = &progress
			}
			if reduced.Commit.State == StateTerminalize {
				terminalEvents++
				combined.State = reduced.Commit.State
				combined.Outcome = reduced.Commit.Outcome
				combined.Run = reduced.Commit.Run
				combined.GoalRun = reduced.Commit.GoalRun
			}
		}
	}
	if terminalEvents != 1 || combined.Run == nil {
		return reductionPublication{}, fmt.Errorf("runs: atomic terminal batch has %d terminal commits", terminalEvents)
	}
	if route.member.ParentID == "" {
		combined.ObsoleteCheckpointRootID = route.member.MemberID
	}
	if err := combined.Validate(); err != nil {
		return reductionPublication{}, fmt.Errorf("runs: validate atomic terminal: %w", err)
	}
	if err := p.coordinator.events.CommitEvent(ctx, combined); err != nil {
		return reductionPublication{}, fmt.Errorf("runs: commit atomic terminal: %w", err)
	}
	p.owner.recordTerminalRun(*combined.Run)
	for _, item := range combined.Items {
		p.owner.recordChildCancellationItem(route.runID, item)
	}
	for _, reduced := range batch.events {
		p.append(route, reduced)
	}
	p.coordinator.publishRunMoved(p.rootSpec.SessionID, route.runID)
	if combined.GoalRun != nil {
		p.coordinator.publishGoalMoved(p.rootSpec.SessionID)
	}
	return reductionPublication{published: true, finished: true}, nil
}

type treeBarrierReduction struct {
	route *executorRoute
	batch reductionBatch
}

type treeBarrierProjection struct {
	pending    Pending
	reductions []treeBarrierReduction
	commits    []EventCommit
}

func (p treePublisher) publishTreeBarrier(
	ctx context.Context,
	routes *executorRoutes,
	barrier TreeInterrupted,
	boundaryAt time.Time,
) (reductionPublication, error) {
	projection, err := p.reduceTreeBarrier(routes, barrier, boundaryAt)
	if err != nil {
		return reductionPublication{}, err
	}
	committed, err := p.owner.commitInterrupt(ctx, func(interruptCtx context.Context) error {
		if err := p.coordinator.barriers.CommitTreeBarrier(interruptCtx, TreeBarrierCommit{
			Pending:    projection.pending,
			Runs:       projection.commits,
			Checkpoint: barrier.Checkpoint,
		}); err != nil {
			return err
		}
		for _, projected := range projection.reductions {
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
	for _, projected := range projection.reductions {
		projected.route.segmentFinished = true
		p.coordinator.publishWaitingMoved(p.rootSpec.SessionID, projected.route.runID)
	}
	return reductionPublication{published: true, finished: true, parked: true}, nil
}

func (p treePublisher) reduceTreeBarrier(
	routes *executorRoutes,
	barrier TreeInterrupted,
	boundaryAt time.Time,
) (treeBarrierProjection, error) {
	if routes == nil || routes.root == nil {
		return treeBarrierProjection{}, errors.New("runs: publish tree barrier without a root executor route")
	}
	if err := barrier.validateFor(
		routes.root.member.MemberID,
		p.rootSpec.SessionID,
		p.rootSpec.GoalLeaseID,
		routes.root.modelSelection,
	); err != nil {
		return treeBarrierProjection{}, err
	}
	interruptionsByMemberID, err := activeInterruptionsByMemberID(routes, barrier.Interruptions)
	if err != nil {
		return treeBarrierProjection{}, err
	}
	activeRoutes, err := routes.unfinishedInPostorder()
	if err != nil {
		return treeBarrierProjection{}, err
	}

	projection := treeBarrierProjection{pending: Pending{
		RootRunID:    routes.root.runID,
		SessionID:    p.rootSpec.SessionID,
		ExecutorID:   p.rootSpec.ExecutorID,
		GoalLeaseID:  p.rootSpec.GoalLeaseID,
		Capabilities: routes.root.capabilities,
		CreatedAt:    boundaryAt,
	},
		reductions: make([]treeBarrierReduction, 0, len(activeRoutes)),
		commits:    make([]EventCommit, 0, len(activeRoutes)),
	}
	for _, route := range activeRoutes {
		directInterruptions := interruptionsByMemberID[route.member.MemberID]
		reduction, bindings, continuation, err := p.reduceInterruptedRoute(
			route,
			directInterruptions,
			boundaryAt,
		)
		if err != nil {
			return treeBarrierProjection{}, err
		}
		projection.pending.Interrupts = append(
			projection.pending.Interrupts,
			reduction.batch.parkCommit.Run.Interrupts...,
		)
		projection.pending.Bindings = append(projection.pending.Bindings, bindings...)
		projection.pending.Continuations = append(projection.pending.Continuations, continuation)
		projection.reductions = append(projection.reductions, reduction)
		projection.commits = append(projection.commits, *reduction.batch.parkCommit)
	}
	if err := projection.pending.Validate(); err != nil {
		return treeBarrierProjection{}, fmt.Errorf("runs: build pending interrupt set: %w", err)
	}
	return projection, nil
}

func activeInterruptionsByMemberID(
	routes *executorRoutes,
	interruptions []MemberInterruption,
) (map[string][]MemberInterruption, error) {
	interruptionsByMemberID := make(map[string][]MemberInterruption, len(interruptions))
	for _, interruption := range interruptions {
		route := routes.byMember[interruption.MemberID]
		if route == nil || route.segmentFinished {
			return nil, fmt.Errorf(
				"runs: request source member %q has no active Run",
				interruption.MemberID,
			)
		}
		interruptionsByMemberID[interruption.MemberID] = append(
			interruptionsByMemberID[interruption.MemberID],
			interruption,
		)
	}
	return interruptionsByMemberID, nil
}

func (p treePublisher) reduceInterruptedRoute(
	route *executorRoute,
	directInterruptions []MemberInterruption,
	boundaryAt time.Time,
) (treeBarrierReduction, []InterruptBinding, Continuation, error) {
	var events []RunEvent
	var err error
	if len(directInterruptions) > 0 {
		interrupts := make([]Interrupt, len(directInterruptions))
		for index, interruption := range directInterruptions {
			interrupts[index] = interruption.Interrupt
		}
		events, err = route.reducer.interrupt(SegmentInterrupted{
			Interrupts: interrupts,
			Duration:   route.activeDuration(boundaryAt),
		})
	} else {
		events, err = route.reducer.suspend(route.activeDuration(boundaryAt))
	}
	if err != nil {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: reduce tree barrier for run %q: %w",
			route.runID,
			err,
		)
	}
	batch, err := route.reducer.project(events)
	if err != nil {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: project tree barrier for run %q: %w",
			route.runID,
			err,
		)
	}
	if err := validateRouteReductionBatch(route, p.rootSpec.SessionID, batch); err != nil {
		return treeBarrierReduction{}, nil, Continuation{}, err
	}
	if batch.parkCommit == nil || batch.parkCommit.Run == nil {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: tree barrier for run %q produced no suspend commit",
			route.runID,
		)
	}
	waitingRun := *batch.parkCommit.Run
	if len(waitingRun.Interrupts) != len(directInterruptions) {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: run %q projected %d interrupts from %d input requests",
			route.runID,
			len(waitingRun.Interrupts),
			len(directInterruptions),
		)
	}
	bindings := make([]InterruptBinding, len(directInterruptions))
	for index, interruption := range directInterruptions {
		bindings[index] = InterruptBinding{
			InterruptItemID: waitingRun.Interrupts[index].ItemID,
			MemberID:        interruption.MemberID,
			RequestID:       interruption.RequestID,
		}
	}
	continuation := Continuation{
		RunID:          route.runID,
		MemberID:       route.member.MemberID,
		Lineage:        route.lineage,
		ModelSelection: route.modelSelection,
		DrainedTools:   slices.Clone(route.reducer.drained),
		CommittedTools: route.reducer.resume.remainingCommittedTools(),
		RunCreatedAt:   waitingRun.CreatedAt,
		Metrics:        waitingRun.Metrics,
		Limits:         waitingRun.Limits,
	}
	return treeBarrierReduction{route: route, batch: batch}, bindings, continuation, nil
}

func (p treePublisher) append(route *executorRoute, reduced reduction) {
	p.owner.hub.append(p.coordinator.event(route.runID, route.segmentID, reduced))
	if reduced.Nudge != nil {
		p.coordinator.workspace.Nudge(reduced.Nudge.CWD, reduced.Nudge.Paths)
	}
	if _, ok := reduced.Event.(StateSnapshot); ok {
		p.coordinator.publishStateMoved(p.rootSpec.SessionID)
	}
}
