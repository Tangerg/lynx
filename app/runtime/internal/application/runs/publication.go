package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/transcript"
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
	for index, reduced := range batch.events {
		if reduced.Event.Terminal() {
			return reductionPublication{}, fmt.Errorf(
				"runs: terminal reduction[%d] requires atomic publication",
				index,
			)
		}
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
		combined.ConversationMessages = appendClonedMessages(
			combined.ConversationMessages,
			reduced.Commit.ConversationMessages...,
		)
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
	combined, err := combineTerminalEventCommit(batch)
	if err != nil {
		return reductionPublication{}, err
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
	// The database owns one indivisible write-set, while the live tree still
	// applies its facts in causal reduction order. Register Item closures before
	// the terminal Run so process-local ownership observes the same ordering.
	for _, item := range combined.Items {
		p.owner.recordChildCancellationItem(route.runID, item)
	}
	p.owner.recordTerminalRun(*combined.Run)
	for _, reduced := range batch.events {
		p.append(route, reduced)
	}
	p.coordinator.publishRunMoved(p.rootSpec.SessionID, route.runID)
	if combined.GoalRun != nil {
		p.coordinator.publishGoalMoved(p.rootSpec.SessionID)
	}
	return reductionPublication{published: true, finished: true}, nil
}

// combineTerminalEventCommit materializes the one write-set that the reducer
// batch represents. Item closures may be projected on events that precede the
// terminal Run event, but they are not independent persistence boundaries:
// terminal state, invocation journals, transcript Items, and accounting either
// all commit or all roll back.
func combineTerminalEventCommit(batch reductionBatch) (EventCommit, error) {
	combined := EventCommit{}
	envelopeSet := false
	terminalCommits := 0
	for index, reduced := range batch.events {
		commit := reduced.Commit
		if commit == nil {
			continue
		}
		if !envelopeSet {
			combined.RunID = commit.RunID
			combined.SessionID = commit.SessionID
			envelopeSet = true
		} else if commit.RunID != combined.RunID || commit.SessionID != combined.SessionID {
			return EventCommit{}, fmt.Errorf(
				"runs: atomic terminal event[%d] changes commit ownership",
				index,
			)
		}
		combined.Items = append(combined.Items, commit.Items...)
		combined.ConversationMessages = appendClonedMessages(
			combined.ConversationMessages,
			commit.ConversationMessages...,
		)
		combined.ModelInvocations = append(combined.ModelInvocations, commit.ModelInvocations...)
		combined.ToolInvocations = append(combined.ToolInvocations, commit.ToolInvocations...)
		if commit.Progress != nil {
			if combined.Progress != nil {
				return EventCommit{}, errors.New("runs: atomic terminal batch repeats Run progress")
			}
			progress := *commit.Progress
			combined.Progress = &progress
		}
		if commit.State == StateTerminalize {
			terminalCommits++
			combined.State = commit.State
			combined.Outcome = commit.Outcome
			combined.Run = commit.Run
			combined.GoalRun = commit.GoalRun
		}
	}
	if terminalCommits != 1 || combined.Run == nil {
		return EventCommit{}, fmt.Errorf(
			"runs: atomic terminal batch has %d terminal commits",
			terminalCommits,
		)
	}
	return combined, nil
}

type treeBarrierReduction struct {
	route      *executorRoute
	batch      reductionBatch
	interrupts []transcript.Interrupt
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
		p.rootSpec.GoalIncarnationID,
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
		RootRunID:         routes.root.runID,
		SessionID:         p.rootSpec.SessionID,
		ExecutorID:        p.rootSpec.ExecutorID,
		GoalIncarnationID: p.rootSpec.GoalIncarnationID,
		Capabilities:      routes.root.capabilities,
		CreatedAt:         boundaryAt,
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
		projection.pending.Interrupts = append(projection.pending.Interrupts, reduction.interrupts...)
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
	var reduced factReduction
	var err error
	if len(directInterruptions) > 0 {
		interrupts := make([]Interrupt, len(directInterruptions))
		for index, interruption := range directInterruptions {
			interrupts[index] = interruption.Interrupt
		}
		reduced, err = route.reducer.interrupt(SegmentInterrupted{
			Interrupts: interrupts,
			Duration:   route.activeDuration(boundaryAt),
		})
	} else {
		reduced, err = route.reducer.suspend(route.activeDuration(boundaryAt))
	}
	if err != nil {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: reduce tree barrier for run %q: %w",
			route.runID,
			err,
		)
	}
	batch, err := route.reducer.projectFact(reduced)
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
	projectedInterrupts := suspendedInterrupts(reduced.events)
	if len(projectedInterrupts) != len(directInterruptions) {
		return treeBarrierReduction{}, nil, Continuation{}, fmt.Errorf(
			"runs: run %q projected %d interrupts from %d input requests",
			route.runID,
			len(projectedInterrupts),
			len(directInterruptions),
		)
	}
	bindings := make([]InterruptBinding, len(directInterruptions))
	for index, interruption := range directInterruptions {
		bindings[index] = InterruptBinding{
			InterruptItemID: projectedInterrupts[index].ItemID,
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
		RunCreatedAt:   waitingRun.CreatedAt(),
		Metrics:        waitingRun.Metrics(),
		Limits:         waitingRun.Limits(),
	}
	return treeBarrierReduction{route: route, batch: batch, interrupts: projectedInterrupts}, bindings, continuation, nil
}

func suspendedInterrupts(events []RunEvent) []transcript.Interrupt {
	for _, event := range events {
		if finished, ok := event.(SegmentFinished); ok {
			return slices.Clone(finished.Interrupts)
		}
	}
	return nil
}

func (p treePublisher) append(route *executorRoute, reduced reduction) {
	event := p.coordinator.event(route.runID, route.segmentID, reduced)
	if route.runID == p.rootSpec.RunID && reduced.Event.Terminal() {
		// The client uses root segment.finished as the stream/completion boundary.
		// Its facts are already durable, but publication must not outrun terminal
		// maintenance and admission release or an immediate next command sees busy.
		p.owner.hub.deferCloseEvent(event)
	} else {
		p.owner.hub.append(event)
	}
	if reduced.Nudge != nil {
		p.coordinator.workspace.Nudge(reduced.Nudge.CWD, reduced.Nudge.Paths)
	}
}
