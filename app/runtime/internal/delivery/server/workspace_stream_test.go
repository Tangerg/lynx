package server

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// subscribe is a broadcast-only convenience used by the workspace-hub tests: a
// hub-owned channel + an unsubscribe that unregisters AND closes it. Production
// subscriptions (SubscribeRuntime) own their channel via register instead, so
// they can close it only after stopping the git watcher — hence this lives in
// test, where the broadcast-only shape is all the tests need.
// allTopics is what a hub test subscribes to: the fan-out is the subject, not the
// per-subscription filter.
func allTopics() map[protocol.RuntimeTopic]bool {
	topics := protocol.RuntimeTopics()
	out := make(map[protocol.RuntimeTopic]bool, len(topics))
	for _, topic := range topics {
		out[topic] = true
	}
	return out
}

// subscribe registers a test consumer for every topic — a hub test is about the
// fan-out, not about what one subscription asked for.
func (h *workspaceHub) subscribe() (<-chan protocol.RuntimeEvent, func()) {
	ch := make(chan protocol.RuntimeEvent, 64)
	_, unregister, ok := h.register(ch, allTopics(), nil)
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
	firstSub, unregisterFirst, _ := hub.register(first, allTopics(), nil)
	_, unregisterSecond, _ := hub.register(second, allTopics(), nil)
	defer unregisterFirst()
	defer unregisterSecond()

	hub.publishTo(firstSub, protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
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

func TestWorkspaceHubScopesResyncToEachSubscriptionsDeclaredTopics(t *testing.T) {
	hub := newWorkspaceHub()
	events := make(chan protocol.RuntimeEvent, 1)
	subscription, unregister, _ := hub.register(events, map[protocol.RuntimeTopic]bool{
		protocol.TopicSkillsChanged: true,
	}, nil)
	defer unregister()

	hub.publishTo(subscription, protocol.RuntimeEvent{
		Type:     protocol.RuntimeResync,
		Topics:   []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		WatchIDs: []string{"foreign-watch"},
	})
	select {
	case event := <-events:
		t.Fatalf("unsubscribed resync leaked into stream: %+v", event)
	default:
	}

	hub.publishTo(subscription, protocol.RuntimeEvent{
		Type: protocol.RuntimeResync,
		Topics: []protocol.RuntimeTopic{
			protocol.TopicFilesChanged,
			protocol.TopicSkillsChanged,
		},
		WatchIDs: []string{"foreign-watch"},
	})
	got := <-events
	if !slices.Equal(got.Topics, []protocol.RuntimeTopic{protocol.TopicSkillsChanged}) {
		t.Fatalf("resync topics = %v, want only skills.changed", got.Topics)
	}
	if len(got.WatchIDs) != 0 {
		t.Fatalf("non-file resync retained foreign watch scope: %v", got.WatchIDs)
	}
}

func TestWorkspaceHubScopesResyncToSubscriptionsDeclaredWatches(t *testing.T) {
	hub := newWorkspaceHub()
	events := make(chan protocol.RuntimeEvent, 2)
	subscription, unregister, _ := hub.register(events, map[protocol.RuntimeTopic]bool{
		protocol.TopicFilesChanged:  true,
		protocol.TopicSkillsChanged: true,
	}, []string{"own-second", "own-first"})
	defer unregister()

	hub.publishTo(subscription, protocol.RuntimeEvent{
		Type: protocol.RuntimeResync,
		Topics: []protocol.RuntimeTopic{
			protocol.TopicFilesChanged,
			protocol.TopicSkillsChanged,
		},
		WatchIDs: []string{"foreign-watch", "own-first", "own-second"},
	})
	got := <-events
	if !slices.Equal(got.Topics, []protocol.RuntimeTopic{
		protocol.TopicFilesChanged,
		protocol.TopicSkillsChanged,
	}) {
		t.Fatalf("resync topics = %v, want files.changed and skills.changed", got.Topics)
	}
	if !slices.Equal(got.WatchIDs, []string{"own-second", "own-first"}) {
		t.Fatalf("resync watch ids = %v, want subscription declaration order", got.WatchIDs)
	}

	hub.publishTo(subscription, protocol.RuntimeEvent{
		Type:     protocol.RuntimeResync,
		Topics:   []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		WatchIDs: []string{"foreign-watch"},
	})
	hub.publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeFilesChanged, WatchID: "foreign-watch", Paths: []string{"foreign.go"},
	})
	select {
	case event := <-events:
		t.Fatalf("foreign watch invalidation leaked into stream: %+v", event)
	default:
	}

	hub.publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeFilesChanged, WatchID: "own-first", Paths: []string{"own.go"},
	})
	if got := <-events; got.Type != protocol.RuntimeFilesChanged || got.WatchID != "own-first" || got.Sequence != 2 {
		t.Fatalf("owned watch invalidation = %+v, want files.changed sequence 2", got)
	}
}

// TestWorkspaceHubCoalescesRatherThanDroppingSignals: a subscriber whose queue is
// full is told WHAT to re-read, not merely that it missed something. The old
// behavior published a number and dropped the frame, which meant a client learned
// about the loss only from a later gap — and on a quiet stream, never. Sequence
// numbers now count only frames actually queued for this subscription.
func TestWorkspaceHubCoalescesRatherThanDroppingSignals(t *testing.T) {
	hub := newWorkspaceHub()
	events := make(chan protocol.RuntimeEvent, 1)
	sub, unregister, _ := hub.register(events, allTopics(), []string{"w1"})
	defer unregister()

	hub.publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeFilesChanged, WatchID: "w1", Paths: []string{"a.go"},
	})
	// Both of these find the queue full and fold into one pending resync.
	hub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged})
	hub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged})

	first := <-events
	if first.Type != protocol.RuntimeFilesChanged || first.Sequence != 1 {
		t.Fatalf("first frame = %s/%d, want files.changed/1", first.Type, first.Sequence)
	}
	// Draining is the only thing that will happen next: no further signal is coming,
	// so the hub has to use the room the consumer just made.
	hub.drained(sub)
	second := <-events
	if second.Type != protocol.RuntimeResync {
		t.Fatalf("second frame = %s, want the coalesced resync", second.Type)
	}
	// Consecutive: the missed signals cost no number, so a gap can only ever mean
	// the transport lost a frame.
	if second.Sequence != 2 {
		t.Fatalf("resync sequence = %d, want 2", second.Sequence)
	}
	if !slices.Equal(second.Topics, []protocol.RuntimeTopic{protocol.TopicSkillsChanged, protocol.TopicSessionsChanged}) {
		t.Fatalf("resync topics = %v, want exactly the two that were held back", second.Topics)
	}
}

// TestWorkspaceHubResyncKeepsTheScopeItWasStalledWith: a resync that cannot be
// delivered must not narrow when it is finally sent — a watch scope it named is a
// read the client still owes.
func TestWorkspaceHubResyncKeepsTheScopeItWasStalledWith(t *testing.T) {
	hub := newWorkspaceHub()
	events := make(chan protocol.RuntimeEvent, 1)
	sub, unregister, _ := hub.register(events, allTopics(), []string{"active-session", "other"})
	defer unregister()

	hub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeMCPChanged})
	hub.publishTo(sub, protocol.RuntimeEvent{
		Type:     protocol.RuntimeResync,
		Topics:   []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		WatchIDs: []string{"active-session"},
	})
	hub.publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeFilesChanged, WatchID: "other", Paths: []string{"other.go"},
	})

	<-events
	hub.drained(sub)
	resync := <-events
	if resync.Type != protocol.RuntimeResync {
		t.Fatalf("frame = %s, want resync", resync.Type)
	}
	if !slices.Equal(resync.Topics, []protocol.RuntimeTopic{protocol.TopicFilesChanged}) {
		t.Fatalf("resync topics = %v, want files.changed", resync.Topics)
	}
	if !slices.Equal(resync.WatchIDs, []string{"active-session", "other"}) {
		t.Fatalf("resync watch ids = %v, want both stalled scopes", resync.WatchIDs)
	}
}

func TestWorkspaceHubBroadFileStallDominatesNarrowWatchScope(t *testing.T) {
	narrow := protocol.RuntimeEvent{
		Type:     protocol.RuntimeResync,
		Topics:   []protocol.RuntimeTopic{protocol.TopicFilesChanged},
		WatchIDs: []string{"own-watch"},
	}
	broad := protocol.RuntimeEvent{
		Type: protocol.RuntimeFilesChanged, Paths: []string{"global.go"},
	}
	for name, signals := range map[string][]protocol.RuntimeEvent{
		"narrow then broad": {narrow, broad},
		"broad then narrow": {broad, narrow},
	} {
		t.Run(name, func(t *testing.T) {
			hub := newWorkspaceHub()
			events := make(chan protocol.RuntimeEvent, 1)
			sub, unregister, _ := hub.register(events, allTopics(), []string{"own-watch"})
			defer unregister()

			hub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeMCPChanged})
			for _, signal := range signals {
				hub.publishTo(sub, signal)
			}

			<-events
			hub.drained(sub)
			resync := <-events
			if !slices.Equal(resync.Topics, []protocol.RuntimeTopic{protocol.TopicFilesChanged}) {
				t.Fatalf("resync topics = %v, want files.changed", resync.Topics)
			}
			if len(resync.WatchIDs) != 0 {
				t.Fatalf("broad file invalidation was incorrectly narrowed to %v", resync.WatchIDs)
			}
		})
	}
}

func TestWorkspaceHubIsolatesMutableEventDataPerSubscription(t *testing.T) {
	hub := newWorkspaceHub()
	first := make(chan protocol.RuntimeEvent, 1)
	second := make(chan protocol.RuntimeEvent, 1)
	_, unregisterFirst, _ := hub.register(first, allTopics(), nil)
	_, unregisterSecond, _ := hub.register(second, allTopics(), nil)
	defer unregisterFirst()
	defer unregisterSecond()

	// Every field of a change signal is a list of ids, so ownership is about slices:
	// the producer must not be able to edit what a subscriber holds, and two
	// subscribers must not share one.
	event := protocol.RuntimeEvent{
		Type:       protocol.RuntimeRunsChanged,
		RunIDs:     []string{"run_1"},
		SessionIDs: []string{"ses_1"},
	}
	hub.publish(event)

	event.RunIDs[0] = "run_mutated"
	event.SessionIDs[0] = "ses_mutated"
	firstEvent := <-first
	secondEvent := <-second
	firstEvent.RunIDs[0] = "run_consumer"
	firstEvent.SessionIDs[0] = "ses_consumer"

	if secondEvent.RunIDs[0] != "run_1" || secondEvent.SessionIDs[0] != "ses_1" {
		t.Fatalf("second subscription observed shared mutable data: %+v", secondEvent)
	}
}

func TestWorkspaceHubRecoversMalformedSignalWithSubscriptionResync(t *testing.T) {
	hub := newWorkspaceHub()
	events := make(chan protocol.RuntimeEvent, 1)
	topics := map[protocol.RuntimeTopic]bool{
		protocol.TopicFilesChanged:  true,
		protocol.TopicSkillsChanged: true,
	}
	_, unregister, _ := hub.register(events, topics, nil)
	defer unregister()

	// A files signal without paths cannot tell the client what changed. Publishing
	// nothing would leave its projection stale, so the hub widens to exactly the
	// resources this subscriber holds.
	hub.publish(protocol.RuntimeEvent{Type: protocol.RuntimeFilesChanged})

	got := <-events
	if got.Type != protocol.RuntimeResync || got.Sequence != 1 {
		t.Fatalf("recovery frame = %+v, want resync sequence 1", got)
	}
	want := []protocol.RuntimeTopic{protocol.TopicFilesChanged, protocol.TopicSkillsChanged}
	if !slices.Equal(got.Topics, want) {
		t.Fatalf("recovery topics = %v, want %v", got.Topics, want)
	}
}

func TestWorkspaceHubRecoversMalformedResyncWithSubscriptionScope(t *testing.T) {
	for name, malformed := range map[string]protocol.RuntimeEvent{
		"missing topics": {
			Type: protocol.RuntimeResync,
		},
		"duplicate topic": {
			Type: protocol.RuntimeResync,
			Topics: []protocol.RuntimeTopic{
				protocol.TopicFilesChanged,
				protocol.TopicFilesChanged,
			},
		},
		"duplicate watch": {
			Type:     protocol.RuntimeResync,
			Topics:   []protocol.RuntimeTopic{protocol.TopicFilesChanged},
			WatchIDs: []string{"own-watch", "own-watch"},
		},
		"unknown topic": {
			Type:   protocol.RuntimeResync,
			Topics: []protocol.RuntimeTopic{"future.changed"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			hub := newWorkspaceHub()
			events := make(chan protocol.RuntimeEvent, 1)
			_, unregister, _ := hub.register(events, map[protocol.RuntimeTopic]bool{
				protocol.TopicFilesChanged:  true,
				protocol.TopicSkillsChanged: true,
			}, []string{"own-watch"})
			defer unregister()

			hub.publish(malformed)
			got := <-events
			wantTopics := []protocol.RuntimeTopic{
				protocol.TopicFilesChanged,
				protocol.TopicSkillsChanged,
			}
			if got.Type != protocol.RuntimeResync || got.Sequence != 1 ||
				!slices.Equal(got.Topics, wantTopics) || len(got.WatchIDs) != 0 {
				t.Fatalf("recovery frame = %+v, want full subscription resync", got)
			}
		})
	}
}

func TestWorkspaceHubCloseLinearizesSubscriptionAdmission(t *testing.T) {
	hub := newWorkspaceHub()
	before := make(chan protocol.RuntimeEvent, 1)
	_, unregister, ok := hub.register(before, allTopics(), nil)
	if !ok {
		t.Fatal("subscription before close was rejected")
	}
	defer unregister()

	hub.closeAdmissions()
	after := make(chan protocol.RuntimeEvent, 1)
	if _, _, ok := hub.register(after, allTopics(), nil); ok {
		t.Fatal("subscription admitted after close returned")
	}

	hub.publish(protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
	if got := <-before; got.Type != protocol.RuntimeResync {
		t.Fatalf("pre-close subscription event = %+v", got)
	}
}
