package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// subscribe is a broadcast-only convenience used by the workspace-hub tests: a
// hub-owned channel + an unsubscribe that unregisters AND closes it. Production
// subscriptions (WorkspaceSubscribe) own their channel via register instead, so
// they can close it only after stopping the git watcher — hence this lives in
// test, where the broadcast-only shape is all the tests need.
func (h *workspaceHub) subscribe() (<-chan protocol.WorkspaceEvent, func()) {
	ch := make(chan protocol.WorkspaceEvent, 64)
	_, unregister := h.register(ch)
	return ch, func() {
		unregister()
		close(ch)
	}
}

func TestWorkspaceHubSequencesEventsPerSubscription(t *testing.T) {
	hub := newWorkspaceHub()
	first := make(chan protocol.WorkspaceEvent, 2)
	second := make(chan protocol.WorkspaceEvent, 1)
	firstSub, unregisterFirst := hub.register(first)
	_, unregisterSecond := hub.register(second)
	defer unregisterFirst()
	defer unregisterSecond()

	hub.publishTo(firstSub, protocol.WorkspaceEvent{Type: protocol.WorkspaceEventResync})
	hub.publish(protocol.WorkspaceEvent{Type: protocol.WorkspaceEventSkillsChanged})

	assertWorkspaceEvent := func(ch <-chan protocol.WorkspaceEvent, wantType protocol.WorkspaceEventType, wantSequence uint64) {
		t.Helper()
		got := <-ch
		if got.Type != wantType || got.Sequence != wantSequence {
			t.Fatalf("event = %+v, want type=%q sequence=%d", got, wantType, wantSequence)
		}
	}
	assertWorkspaceEvent(first, protocol.WorkspaceEventResync, 1)
	assertWorkspaceEvent(first, protocol.WorkspaceEventSkillsChanged, 2)
	assertWorkspaceEvent(second, protocol.WorkspaceEventSkillsChanged, 1)
}

func TestWorkspaceHubSequenceExposesDroppedEvent(t *testing.T) {
	hub := newWorkspaceHub()
	events := make(chan protocol.WorkspaceEvent, 1)
	_, unregister := hub.register(events)
	defer unregister()

	hub.publish(protocol.WorkspaceEvent{Type: "first"})
	hub.publish(protocol.WorkspaceEvent{Type: "dropped"})
	if got := <-events; got.Sequence != 1 {
		t.Fatalf("first sequence = %d, want 1", got.Sequence)
	}
	hub.publish(protocol.WorkspaceEvent{Type: "third"})
	if got := <-events; got.Sequence != 3 {
		t.Fatalf("sequence after drop = %d, want 3", got.Sequence)
	}
}

func TestWorkspaceHubIsolatesMutableEventDataPerSubscription(t *testing.T) {
	hub := newWorkspaceHub()
	first := make(chan protocol.WorkspaceEvent, 1)
	second := make(chan protocol.WorkspaceEvent, 1)
	_, unregisterFirst := hub.register(first)
	_, unregisterSecond := hub.register(second)
	defer unregisterFirst()
	defer unregisterSecond()

	toolCount := 2
	event := protocol.WorkspaceEvent{
		Type:      protocol.WorkspaceEventFilesChanged,
		Paths:     []string{"a.go"},
		ToolCount: &toolCount,
		Error: &protocol.ProblemData{
			Type:   protocol.ProblemToolFailed,
			Errors: []protocol.FieldError{{Field: "path", Detail: "invalid"}},
		},
	}
	hub.publish(event)

	event.Paths[0] = "producer-mutated.go"
	*event.ToolCount = 9
	event.Error.Errors[0].Field = "producer-mutated"
	firstEvent := <-first
	secondEvent := <-second
	firstEvent.Paths[0] = "consumer-mutated.go"
	*firstEvent.ToolCount = 7
	firstEvent.Error.Errors[0].Field = "consumer-mutated"

	if secondEvent.Paths[0] != "a.go" || *secondEvent.ToolCount != 2 || secondEvent.Error.Errors[0].Field != "path" {
		t.Fatalf("second subscription observed shared mutable data: %+v", secondEvent)
	}
}
