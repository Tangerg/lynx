package server

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

// TestEveryChangeResourceIsPublishable: the application's resource set and the wire
// topics are two spellings of one vocabulary, and the projection between them is
// hand-written. A resource with no mapping would be a committed change no client is
// ever told about — silent, and invisible to every other test.
func TestEveryChangeResourceIsPublishable(t *testing.T) {
	for _, resource := range change.Resources {
		ev, ok := runtimeEventFor(change.Notice{Resource: resource, SessionIDs: []string{"ses_1"}})
		if !ok {
			t.Fatalf("resource %d has no runtime event", resource)
		}
		if !slices.Contains(protocol.RuntimeTopics, protocol.RuntimeTopic(ev.Type)) {
			t.Fatalf("resource %d maps to %q, which is not a subscribable topic", resource, ev.Type)
		}
		if !slices.Equal(ev.SessionIDs, []string{"ses_1"}) {
			t.Fatalf("resource %d dropped the session scope: %+v", resource, ev.SessionIDs)
		}
	}
}

// TestStateChangeNamesItsKeyAndKeepsSessionScope: state.changed is the one signal
// with a required field beyond the sequence — without the key a client cannot tell
// which recovery method to call — and a session-scoped key must not carry run ids,
// which would narrow a refetch by something the key is not keyed on.
//
// It is the wire half of session_state_is_owned_by_its_session and of
// committed_state_change_reaches_other_windows: the store keeps one value per
// session, and this is where that scope survives being published.
func TestStateChangeNamesItsKeyAndKeepsSessionScope(t *testing.T) {
	ev, ok := runtimeEventFor(change.Notice{
		Resource: change.PlanState, SessionIDs: []string{"ses_1"}, RunIDs: []string{"run_1"},
	})
	if !ok {
		t.Fatal("plan state has no runtime event")
	}
	if ev.Type != protocol.RuntimeStateChanged || ev.Key != protocol.StatePlan {
		t.Fatalf("event = %s key %q, want state.changed/plan", ev.Type, ev.Key)
	}
	if len(ev.RunIDs) != 0 {
		t.Fatalf("run ids = %v, want none on a session-scoped key", ev.RunIDs)
	}
}
