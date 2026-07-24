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
		outcome := presentOutcome(event.Run)
		return protocol.StreamEvent{Type: protocol.StreamSegmentFinished, Outcome: &outcome}
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
		// presentRunEvent asserts an exhaustive event switch and dereferences
		// payloads; it runs synchronously on the consumer's goroutine, outside the
		// request-scoped panic recovery. Contain a panic to this one stream — record
		// it and end the sequence — so an unrecovered presenter bug terminates this
		// single stream rather than aborting the whole process.
		defer func() {
			if r := recover(); r != nil {
				trace.SpanFromContext(ctx).RecordError(fmt.Errorf("server: run-event presenter panicked, terminating stream: %v", r))
			}
		}()
		for event := range in {
			wire := protocol.RunEvent{
				RunID: event.RunID, SegmentID: event.SegmentID,
				EventID: protocol.IDPrefixEvent + event.Seq, Timestamp: event.Timestamp,
				Event: presentRunEvent(event.Payload),
			}
			if !yield(wire) {
				return
			}
		}
	}
}
