package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// subscribe is a broadcast-only convenience used by the workspace-hub tests: a
// hub-owned channel + an unsubscribe that unregisters AND closes it. Production
// subscriptions (SubscribeRuntime) own their channel via register instead, so
// they can close it only after stopping the git watcher — hence this lives in
// test, where the broadcast-only shape is all the tests need.
// allTopics is what a hub test subscribes to: the fan-out is the subject, not the
// per-subscription filter.
func allTopics() map[protocol.RuntimeTopic]bool {
	out := make(map[protocol.RuntimeTopic]bool, len(protocol.RuntimeTopics))
	for _, topic := range protocol.RuntimeTopics {
		out[topic] = true
	}
	return out
}

// subscribe registers a test consumer for every topic — a hub test is about the
// fan-out, not about what one subscription asked for.
func (h *workspaceHub) subscribe() (<-chan protocol.RuntimeEvent, func()) {
	ch := make(chan protocol.RuntimeEvent, 64)
	_, unregister, ok := h.register(ch, allTopics())
	if !ok {
		close(ch)
		return ch, func() {}
	}
	return ch, func() {
		unregister()
		close(ch)
	}
}

func TestWorkspaceHubSequencesEventsPerSubscription(t *testing.T) {
	hub := newWorkspaceHub()
	first := make(chan protocol.RuntimeEvent, 2)
	second := make(chan protocol.RuntimeEvent, 1)
	firstSub, unregisterFirst, _ := hub.register(first, allTopics())
	_, unregisterSecond, _ := hub.register(second, allTopics())
	defer unregisterFirst()
	defer unregisterSecond()

	hub.publishTo(firstSub, protocol.RuntimeEvent{Type: protocol.RuntimeResync})
	hub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged})

	assertWorkspaceEvent := func(ch <-chan protocol.RuntimeEvent, wantType protocol.RuntimeEventType, wantSequence uint64) {
		t.Helper()
		got := <-ch
		if got.Type != wantType || got.Sequence != wantSequence {
			t.Fatalf("event = %+v, want type=%q sequence=%d", got, wantType, wantSequence)
		}
	}
	assertWorkspaceEvent(first, protocol.RuntimeResync, 1)
	assertWorkspaceEvent(first, protocol.RuntimeSkillsChanged, 2)
	assertWorkspaceEvent(second, protocol.RuntimeSkillsChanged, 1)
}

func TestWorkspaceHubSequenceExposesDroppedEvent(t *testing.T) {
	hub := newWorkspaceHub()
	events := make(chan protocol.RuntimeEvent, 1)
	_, unregister, _ := hub.register(events, allTopics())
	defer unregister()

	hub.publish(protocol.RuntimeEvent{Type: "first"})
	hub.publish(protocol.RuntimeEvent{Type: "dropped"})
	if got := <-events; got.Sequence != 1 {
		t.Fatalf("first sequence = %d, want 1", got.Sequence)
	}
	hub.publish(protocol.RuntimeEvent{Type: "third"})
	if got := <-events; got.Sequence != 3 {
		t.Fatalf("sequence after drop = %d, want 3", got.Sequence)
	}
}

func TestWorkspaceHubIsolatesMutableEventDataPerSubscription(t *testing.T) {
	hub := newWorkspaceHub()
	first := make(chan protocol.RuntimeEvent, 1)
	second := make(chan protocol.RuntimeEvent, 1)
	_, unregisterFirst, _ := hub.register(first, allTopics())
	_, unregisterSecond, _ := hub.register(second, allTopics())
	defer unregisterFirst()
	defer unregisterSecond()

	// Every field of a change signal is a list of ids, so ownership is about slices:
	// the producer must not be able to edit what a subscriber holds, and two
	// subscribers must not share one.
	event := protocol.RuntimeEvent{
		Type:       protocol.RuntimeFilesChanged,
		Paths:      []string{"a.go"},
		SessionIDs: []string{"ses_1"},
	}
	hub.publish(event)

	event.Paths[0] = "producer-mutated.go"
	event.SessionIDs[0] = "ses_mutated"
	firstEvent := <-first
	secondEvent := <-second
	firstEvent.Paths[0] = "consumer-mutated.go"
	firstEvent.SessionIDs[0] = "ses_consumer"

	if secondEvent.Paths[0] != "a.go" || secondEvent.SessionIDs[0] != "ses_1" {
		t.Fatalf("second subscription observed shared mutable data: %+v", secondEvent)
	}
}

func TestWorkspaceHubCloseLinearizesSubscriptionAdmission(t *testing.T) {
	hub := newWorkspaceHub()
	before := make(chan protocol.RuntimeEvent, 1)
	_, unregister, ok := hub.register(before, allTopics())
	if !ok {
		t.Fatal("subscription before close was rejected")
	}
	defer unregister()

	hub.closeAdmissions()
	after := make(chan protocol.RuntimeEvent, 1)
	if _, _, ok := hub.register(after, allTopics()); ok {
		t.Fatal("subscription admitted after close returned")
	}

	hub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeResync})
	if got := <-before; got.Type != protocol.RuntimeResync {
		t.Fatalf("pre-close subscription event = %+v", got)
	}
}
