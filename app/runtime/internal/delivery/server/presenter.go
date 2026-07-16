package server

import (
	"context"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
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
				ID: todo.ID, Text: todo.Text, Status: todo.Status,
				BlockedReason: todo.BlockedReason, NextAction: todo.NextAction,
			}
		}
		return protocol.StreamEvent{Type: protocol.StreamStateSnapshot, State: map[string]any{"todos": todos}}
	default:
		panic("server: unknown canonical run event")
	}
}

func mapRunEvents(ctx context.Context, in <-chan runs.Event) <-chan protocol.RunEvent {
	out := make(chan protocol.RunEvent)
	go func() {
		defer close(out)
		for event := range in {
			wire := protocol.RunEvent{
				RunID: event.RunID, SegmentID: event.SegmentID,
				EventID: protocol.IDPrefixEvent + event.Seq, Timestamp: event.Timestamp,
				Event: presentRunEvent(event.Payload),
			}
			select {
			case out <- wire:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}
