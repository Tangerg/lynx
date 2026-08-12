package changefeed

import (
	"slices"
	"strings"
	"testing"
)

func TestTopicsReturnsAnOwnedCompleteInventory(t *testing.T) {
	t.Parallel()
	want := []Topic{
		FilesChanged, SkillsChanged, MCPChanged, SchedulesChanged,
		SessionsChanged, RunsChanged, StateChanged, GoalsChanged, InterruptsChanged,
		KnowledgeChanged, HooksChanged,
	}
	got := Topics()
	if !slices.Equal(got, want) {
		t.Fatalf("Topics = %v, want %v", got, want)
	}
	got[0] = "mutated"
	if Topics()[0] != FilesChanged {
		t.Fatal("mutating a Topics result rewrote the package inventory")
	}
}

func TestSubscriptionMakesWatchScopeExplicit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		subscription Subscription
		want         string
	}{
		{name: "valid", subscription: Subscription{Topics: []Topic{FilesChanged}, Watches: []Watch{{ID: "active", Workspace: "/workspace"}}}},
		{name: "watch without topic", subscription: Subscription{Topics: []Topic{RunsChanged}, Watches: []Watch{{ID: "active", Workspace: "/workspace"}}}, want: "files.changed"},
		{name: "duplicate topic", subscription: Subscription{Topics: []Topic{FilesChanged, FilesChanged}}, want: "repeats"},
		{name: "duplicate watch", subscription: Subscription{Topics: []Topic{FilesChanged}, Watches: []Watch{{ID: "active", Workspace: "/workspace"}, {ID: "active", Workspace: "/other"}}}, want: "repeats"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := test.subscription.Validate()
			if test.want == "" && err != nil {
				t.Fatalf("Validate: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("Validate = %v, want %q", err, test.want)
			}
		})
	}
}

func TestEventDistinguishesInvalidationFromResync(t *testing.T) {
	t.Parallel()
	changed := Event{Type: EventType(FilesChanged), Sequence: 1, WatchID: "active", Workspace: "/workspace", Paths: []string{"main.go"}}
	if err := changed.Validate(); err != nil {
		t.Fatal(err)
	}
	resync := Event{Type: Resync, Sequence: 2, Topics: []Topic{FilesChanged}}
	if err := resync.Validate(); err != nil {
		t.Fatal(err)
	}
	resync.Topics = nil
	if err := resync.Validate(); err == nil {
		t.Fatal("scope-free resync was accepted")
	}
}

func TestStateChangeRequiresTheProjectionThisClientOwns(t *testing.T) {
	t.Parallel()
	event := Event{Type: EventType(StateChanged), Sequence: 1, StateKey: StatePlan}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
	event.StateKey = "vendor-state"
	if err := event.Validate(); err == nil {
		t.Fatal("unsupported state projection was accepted")
	}
}
