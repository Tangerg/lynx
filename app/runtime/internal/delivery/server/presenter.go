package server

import (
	"context"
	"fmt"
	"iter"
	"strconv"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
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
		outcome, metrics := presentSegmentFinished(event.Run)
		return protocol.StreamEvent{Type: protocol.StreamSegmentFinished, Outcome: &outcome, Metrics: &metrics}
	case runs.ItemStarted:
		item := presentItem(event.Item)
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

// presentStateSnapshot publishes what a run changed. The stream and todos.get go
// through one shape, so "recover this key" cannot mean something different from
// "follow this key".
func presentStateSnapshot(event runs.StateSnapshot) protocol.StateSnapshot {
	todos := make([]protocol.TodoSnapshot, 0, len(event.Todos))
	for _, item := range event.Todos {
		todos = append(todos, protocol.TodoSnapshot{
			ID: item.ID, Text: item.Text, Status: presentTodoStatus(item.Status),
			BlockedReason: item.BlockedReason, NextAction: item.NextAction,
		})
	}
	return protocol.StateSnapshot{
		Type: protocol.StateTodos, SessionID: event.SessionID,
		Revision: event.Revision, Todos: todos, UpdatedAt: event.UpdatedAt,
	}
}

// presentTodoState is the same projection read cold. It goes through the run-event
// shape so the two cannot describe the list differently: one presenter, one answer.
func presentTodoState(sessionID string, state todo.State) protocol.StateSnapshot {
	snapshot := runs.StateSnapshot{
		SessionID: sessionID, Revision: state.Revision, UpdatedAt: state.UpdatedAt,
		Todos: make([]runs.TodoSnapshot, 0, len(state.Items)),
	}
	for index, item := range state.Items {
		snapshot.Todos = append(snapshot.Todos, runs.TodoSnapshot{
			ID: strconv.Itoa(index), Text: item.Content, Status: item.Status,
			BlockedReason: item.BlockedReason, NextAction: item.NextAction,
		})
	}
	return presentStateSnapshot(snapshot)
}

func presentTodoStatus(status todo.Status) protocol.TodoStatus {
	switch status {
	case todo.StatusPending:
		return protocol.TodoStatusPending
	case todo.StatusInProgress:
		return protocol.TodoStatusInProgress
	case todo.StatusCompleted:
		return protocol.TodoStatusCompleted
	default:
		panic("server: unknown todo status")
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
				EventID: protocol.IDPrefixEvent + event.Seq, Timestamp: event.Timestamp,
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
