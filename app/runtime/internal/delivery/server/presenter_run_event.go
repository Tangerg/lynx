package server

import (
	"context"
	"fmt"
	"iter"
	"strconv"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/scope/app/runtime/internal/application/runs"
	"github.com/Tangerg/scope/app/runtime/internal/domain/plan"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

func presentRunEvent(event runs.RunEvent) protocol.StreamEvent {
	switch event := event.(type) {
	case runs.SegmentStarted:
		run := presentRun(event.Run)
		return protocol.StreamEvent{Type: protocol.StreamSegmentStarted, Run: &run}
	case runs.SegmentProgressed:
		progress := presentProgress(event.Progress)
		return protocol.StreamEvent{Type: protocol.StreamSegmentProgress, Progress: &progress}
	case runs.SegmentFinished:
		outcome, metrics := presentSegmentFinished(event.Run, event.Interrupts)
		return protocol.StreamEvent{Type: protocol.StreamSegmentFinished, Outcome: &outcome, Metrics: &metrics}
	case runs.ItemStarted:
		item := presentItemStart(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemStarted, Item: &item}
	case runs.ItemChanged:
		delta := presentDelta(event.Delta)
		return protocol.StreamEvent{Type: protocol.StreamItemDelta, ItemID: event.ItemID, Delta: &delta}
	case runs.ItemCompleted:
		item := presentItem(event.Item)
		return protocol.StreamEvent{Type: protocol.StreamItemCompleted, Item: &item}
	case runs.PlanSnapshot:
		plan := presentPlan(event)
		return protocol.StreamEvent{Type: protocol.StreamPlanUpdated, Plan: &plan}
	default:
		panic("server: unknown canonical run event")
	}
}

// presentPlan publishes what a root Run changed. The stream and plan.get go through
// one shape, so live following and cold recovery cannot disagree about the Plan.
func presentPlan(event runs.PlanSnapshot) protocol.Plan {
	steps := make([]protocol.PlanStep, 0, len(event.Steps))
	for index, step := range event.Steps {
		steps = append(steps, protocol.PlanStep{
			ID: strconv.Itoa(index), Description: step.Description, Status: presentPlanStatus(step.Status),
		})
	}
	return protocol.Plan{
		SessionID: event.SessionID, Revision: event.Revision,
		Steps: steps, UpdatedAt: event.UpdatedAt,
	}
}

// presentStoredPlan is the same projection read cold. It goes through the run-event
// shape so the two cannot describe the list differently: one presenter, one answer.
func presentStoredPlan(sessionID string, state plan.State) protocol.Plan {
	return presentPlan(runs.PlanSnapshot{
		SessionID: sessionID, Revision: state.Revision(), UpdatedAt: state.UpdatedAt(),
		Steps: state.Steps(),
	})
}

// presentPlanSteps is the list a portable archive carries: the same items as
// the live projection, through the same presenter, with none of the revision or
// timestamp the archive deliberately leaves behind.
func presentPlanSteps(steps []plan.Step) []protocol.PlanStep {
	return presentPlan(runs.PlanSnapshot{Steps: steps}).Steps
}

func presentPlanStatus(status plan.Status) protocol.PlanStatus {
	switch status {
	case plan.StatusPending:
		return protocol.PlanStatusPending
	case plan.StatusInProgress:
		return protocol.PlanStatusInProgress
	case plan.StatusCompleted:
		return protocol.PlanStatusCompleted
	default:
		panic("server: unknown plan status")
	}
}

func mapRunEvents(ctx context.Context, in iter.Seq[runs.Event]) iter.Seq[protocol.RunEvent] {
	return func(yield func(protocol.RunEvent) bool) {
		for event := range in {
			presented, ok := safePresentRunEvent(ctx, event.Payload)
			if !ok {
				return
			}
			wire := protocol.RunEvent{
				RunID: event.RunID, SegmentID: event.SegmentID,
				EventID: protocol.IDPrefixEvent + event.Cursor, Timestamp: event.Timestamp,
				Event: presented,
			}
			if !yield(wire) {
				return
			}
		}
	}
}

// safePresentRunEvent contains only presenter failures. In particular, it must
// not recover a panic raised by the downstream range body through yield.
func safePresentRunEvent(ctx context.Context, event runs.RunEvent) (presented protocol.StreamEvent, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			trace.SpanFromContext(ctx).RecordError(fmt.Errorf("server: run-event presenter panicked, terminating stream: %v", r))
			presented = protocol.StreamEvent{}
			ok = false
		}
	}()
	return presentRunEvent(event), true
}
