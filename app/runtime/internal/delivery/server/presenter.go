package server

import (
	"context"
	"fmt"
	"iter"

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
		todos := make([]protocol.TodoSnapshot, len(event.Todos))
		for i, todo := range event.Todos {
			todos[i] = protocol.TodoSnapshot{
				ID: todo.ID, Text: todo.Text, Status: presentTodoStatus(todo.Status),
				BlockedReason: todo.BlockedReason, NextAction: todo.NextAction,
			}
		}
		return protocol.StreamEvent{Type: protocol.StreamStateSnapshot, State: map[string]any{"todos": todos}}
	default:
		panic("server: unknown canonical run event")
	}
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
