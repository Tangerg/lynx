package server

import (
	"context"
	"fmt"
	"iter"
	"strconv"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/protocol"
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
	case runs.StateSnapshot:
		state := presentStateSnapshot(event)
		return protocol.StreamEvent{Type: protocol.StreamStateSnapshot, State: &state}
	default:
		panic("server: unknown canonical run event")
	}
}

// presentStateSnapshot publishes what a run changed. The stream and plan.get go
// through one shape, so "recover this key" cannot mean something different from
// "follow this key".
func presentStateSnapshot(event runs.StateSnapshot) protocol.StateSnapshot {
	plan := make([]protocol.PlanSnapshot, 0, len(event.Plan))
	for _, step := range event.Plan {
		plan = append(plan, protocol.PlanSnapshot{
			ID: step.ID, Description: step.Description, Status: presentPlanStatus(step.Status),
		})
	}
	return protocol.StateSnapshot{
		Type: protocol.StatePlan, SessionID: event.SessionID,
		Revision: event.Revision, Plan: plan, UpdatedAt: event.UpdatedAt,
	}
}

// presentPlanState is the same projection read cold. It goes through the run-event
// shape so the two cannot describe the list differently: one presenter, one answer.
func presentPlanState(sessionID string, state plan.State) protocol.StateSnapshot {
	return presentStateSnapshot(runs.StateSnapshot{
		SessionID: sessionID, Revision: state.Revision(), UpdatedAt: state.UpdatedAt(),
		Plan: planSnapshots(state.Steps()),
	})
}

// presentPlanSnapshots is the list a portable archive carries: the same items as
// the live projection, through the same presenter, with none of the revision or
// timestamp the archive deliberately leaves behind.
func presentPlanSnapshots(steps []plan.Step) []protocol.PlanSnapshot {
	return presentStateSnapshot(runs.StateSnapshot{Plan: planSnapshots(steps)}).Plan
}

// planSnapshots numbers the items by position, which is what a Plan's
// identity IS: the model replaces the whole list, so an item is the nth entry
// rather than a thing with a durable id.
func planSnapshots(steps []plan.Step) []runs.PlanSnapshot {
	out := make([]runs.PlanSnapshot, 0, len(steps))
	for index, step := range steps {
		out = append(out, runs.PlanSnapshot{
			ID: strconv.Itoa(index), Description: step.Description, Status: step.Status,
		})
	}
	return out
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
