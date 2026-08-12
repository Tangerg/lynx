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

func TestSubscriptionLimitsPartitionWithoutLosingDeliveryScope(t *testing.T) {
	t.Parallel()
	requested := Subscription{
		Topics: []Topic{FilesChanged, SessionsChanged, RunsChanged, StateChanged, InterruptsChanged},
		Watches: []Watch{
			{ID: "first", Workspace: "/first"},
			{ID: "second", Workspace: "/second"},
			{ID: "third", Workspace: "/third"},
		},
	}
	partitions, err := (SubscriptionLimits{MaxTopics: 2, MaxWatches: 2}).Partition(requested)
	if err != nil {
		t.Fatal(err)
	}
	want := []Subscription{
		{Topics: []Topic{FilesChanged, SessionsChanged}, Watches: requested.Watches[:2]},
		{Topics: []Topic{RunsChanged, StateChanged}},
		{Topics: []Topic{InterruptsChanged}},
		{Topics: []Topic{FilesChanged}, Watches: requested.Watches[2:]},
	}
	if !slices.EqualFunc(partitions, want, func(got, want Subscription) bool {
		return slices.Equal(got.Topics, want.Topics) && slices.Equal(got.Watches, want.Watches)
	}) {
		t.Fatalf("partitions = %+v, want %+v", partitions, want)
	}
	partitions[0].Topics[0] = RunsChanged
	partitions[0].Watches[0].ID = "mutated"
	if requested.Topics[0] != FilesChanged || requested.Watches[0].ID != "first" {
		t.Fatal("partition returned aliases into the requested subscription")
	}
}

func TestSubscriptionLimitsKeepAnUnboundedRequestWhole(t *testing.T) {
	t.Parallel()
	requested := Subscription{
		Topics:  []Topic{FilesChanged, SessionsChanged},
		Watches: []Watch{{ID: "active", Workspace: "/workspace"}},
	}
	partitions, err := (SubscriptionLimits{}).Partition(requested)
	if err != nil {
		t.Fatal(err)
	}
	if len(partitions) != 1 || !slices.Equal(partitions[0].Topics, requested.Topics) ||
		!slices.Equal(partitions[0].Watches, requested.Watches) {
		t.Fatalf("partitions = %+v, want one complete subscription", partitions)
	}
}

func TestSubscriptionLimitsRejectInvalidConstraints(t *testing.T) {
	t.Parallel()
	requested := Subscription{Topics: []Topic{SessionsChanged}}
	for _, limits := range []SubscriptionLimits{{MaxTopics: -1}, {MaxWatches: -1}} {
		if _, err := limits.Partition(requested); err == nil {
			t.Fatalf("negative limits %+v were accepted", limits)
		}
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

func TestEventAcceptsBroadFileInvalidations(t *testing.T) {
	t.Parallel()
	event := Event{
		Type: EventType(FilesChanged), Sequence: 1,
		Workspace: "/workspace", Paths: []string{"main.go"},
	}
	if err := event.Validate(); err != nil {
		t.Fatalf("broad file invalidation: %v", err)
	}
	event.Paths = nil
	if err := event.Validate(); err == nil {
		t.Fatal("pathless file invalidation was accepted")
	}
}

func TestSubscriptionRejectsEventsOutsideItsDeclaredScope(t *testing.T) {
	t.Parallel()
	subscription := Subscription{
		Topics:  []Topic{FilesChanged, SessionsChanged},
		Watches: []Watch{{ID: "active", Workspace: "/workspace"}},
	}
	tests := []struct {
		name  string
		event Event
		want  string
	}{
		{
			name: "broad file event", event: Event{
				Type: EventType(FilesChanged), Sequence: 1,
				Workspace: "/workspace", Paths: []string{"main.go"},
			},
		},
		{
			name: "owned watch", event: Event{
				Type: EventType(FilesChanged), Sequence: 1, WatchID: "active",
				Workspace: "/workspace", Paths: []string{"main.go"},
			},
		},
		{
			name: "foreign topic", want: "outside the subscription",
			event: Event{Type: EventType(RunsChanged), Sequence: 1},
		},
		{
			name: "foreign watch", want: "outside the subscription",
			event: Event{
				Type: EventType(FilesChanged), Sequence: 1, WatchID: "foreign",
				Workspace: "/workspace", Paths: []string{"main.go"},
			},
		},
		{
			name: "watch workspace mismatch", want: "another workspace",
			event: Event{
				Type: EventType(FilesChanged), Sequence: 1, WatchID: "active",
				Workspace: "/other", Paths: []string{"main.go"},
			},
		},
		{
			name: "foreign resync topic", want: "outside the subscription",
			event: Event{Type: Resync, Sequence: 1, Topics: []Topic{RunsChanged}},
		},
		{
			name: "foreign resync watch", want: "outside the subscription",
			event: Event{
				Type: Resync, Sequence: 1, Topics: []Topic{FilesChanged}, WatchIDs: []string{"foreign"},
			},
		},
		{
			name: "watch without file resync", want: "files.changed",
			event: Event{
				Type: Resync, Sequence: 1, Topics: []Topic{SessionsChanged}, WatchIDs: []string{"active"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := subscription.ValidateEvent(test.event)
			if test.want == "" && err != nil {
				t.Fatalf("ValidateEvent: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("ValidateEvent = %v, want %q", err, test.want)
			}
		})
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
