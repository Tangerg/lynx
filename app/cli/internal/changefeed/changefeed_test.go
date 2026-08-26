package changefeed

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestTopicsReturnsAnOwnedCompleteInventory(t *testing.T) {
	t.Parallel()
	want := []Topic{
		FilesChanged, SkillsChanged, MCPChanged, SchedulesChanged,
		SessionsChanged, RunsChanged, PlanChanged, GoalsChanged, InterruptsChanged,
		KnowledgeChanged, HooksChanged, ModelsChanged, ApprovalsChanged,
		AgentMemoryChanged,
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
		Topics: []Topic{FilesChanged, SessionsChanged, RunsChanged, PlanChanged, InterruptsChanged},
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
		{Topics: []Topic{RunsChanged, PlanChanged}},
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

func TestSubscriptionLimitsKeepWorkspaceObservationAtomicAcrossTopicPartitions(t *testing.T) {
	t.Parallel()
	requested := Subscription{
		Topics: []Topic{
			FilesChanged, SessionsChanged, RunsChanged, KnowledgeChanged, HooksChanged,
		},
		Watches: []Watch{{ID: "active", Workspace: "/workspace"}},
	}
	partitions, err := (SubscriptionLimits{MaxTopics: 2, MaxWatches: 1}).Partition(requested)
	if err != nil {
		t.Fatal(err)
	}
	want := []Subscription{
		{Topics: []Topic{FilesChanged, KnowledgeChanged}, Watches: requested.Watches},
		{Topics: []Topic{FilesChanged, HooksChanged}, Watches: requested.Watches},
		{Topics: []Topic{SessionsChanged, RunsChanged}},
	}
	if !slices.EqualFunc(partitions, want, func(got, want Subscription) bool {
		return slices.Equal(got.Topics, want.Topics) && slices.Equal(got.Watches, want.Watches)
	}) {
		t.Fatalf("partitions = %+v, want %+v", partitions, want)
	}
}

func TestSubscriptionLimitsRepeatWorkspaceObservationForEveryWatchPartition(t *testing.T) {
	t.Parallel()
	requested := Subscription{
		Topics: []Topic{FilesChanged, KnowledgeChanged, HooksChanged},
		Watches: []Watch{
			{ID: "first", Workspace: "/first"},
			{ID: "second", Workspace: "/second"},
		},
	}
	partitions, err := (SubscriptionLimits{MaxTopics: 3, MaxWatches: 1}).Partition(requested)
	if err != nil {
		t.Fatal(err)
	}
	want := []Subscription{
		{Topics: requested.Topics, Watches: requested.Watches[:1]},
		{Topics: requested.Topics, Watches: requested.Watches[1:]},
	}
	if !slices.EqualFunc(partitions, want, func(got, want Subscription) bool {
		return slices.Equal(got.Topics, want.Topics) && slices.Equal(got.Watches, want.Watches)
	}) {
		t.Fatalf("partitions = %+v, want %+v", partitions, want)
	}
}

func TestSubscriptionLimitsRejectUnrepresentableWorkspaceObservation(t *testing.T) {
	t.Parallel()
	requested := Subscription{
		Topics:  []Topic{FilesChanged, KnowledgeChanged},
		Watches: []Watch{{ID: "active", Workspace: "/workspace"}},
	}
	if _, err := (SubscriptionLimits{MaxTopics: 1, MaxWatches: 1}).Partition(requested); err == nil ||
		!strings.Contains(err.Error(), "workspace observation") {
		t.Fatalf("Partition error = %v, want workspace observation failure", err)
	}
}

func TestSubscriptionLimitsPreserveEveryDeliveryInvariant(t *testing.T) {
	t.Parallel()
	optional := []Topic{SessionsChanged, RunsChanged, KnowledgeChanged, HooksChanged}
	for mask := 0; mask < 1<<len(optional); mask++ {
		topics := []Topic{FilesChanged}
		for index, topic := range optional {
			if mask&(1<<index) != 0 {
				topics = append(topics, topic)
			}
		}
		for watchCount := 1; watchCount <= 3; watchCount++ {
			watches := make([]Watch, watchCount)
			for index := range watches {
				watches[index] = Watch{ID: fmt.Sprintf("watch-%d", index), Workspace: fmt.Sprintf("/workspace/%d", index)}
			}
			requested := Subscription{Topics: topics, Watches: watches}
			for topicLimit := 1; topicLimit <= len(topics); topicLimit++ {
				for watchLimit := 1; watchLimit <= watchCount; watchLimit++ {
					name := fmt.Sprintf("mask-%02x/watches-%d/topics-%d/watch-limit-%d", mask, watchCount, topicLimit, watchLimit)
					t.Run(name, func(t *testing.T) {
						partitions, err := (SubscriptionLimits{MaxTopics: topicLimit, MaxWatches: watchLimit}).Partition(requested)
						observed := workspaceObservedTopics(topics)
						if len(observed) > 0 && topicLimit == 1 {
							if err == nil {
								t.Fatalf("unrepresentable workspace observation produced %+v", partitions)
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						assertPartitionDeliveryInvariants(t, requested, partitions, topicLimit, watchLimit)
					})
				}
			}
		}
	}
}

func assertPartitionDeliveryInvariants(
	t *testing.T,
	requested Subscription,
	partitions []Subscription,
	topicLimit int,
	watchLimit int,
) {
	t.Helper()
	if len(partitions) == 0 {
		t.Fatal("partitioning returned no subscriptions")
	}
	for index, partition := range partitions {
		if err := partition.Validate(); err != nil {
			t.Fatalf("partition %d is invalid: %v", index, err)
		}
		if len(partition.Topics) > topicLimit || len(partition.Watches) > watchLimit {
			t.Fatalf("partition %d exceeds limits: %+v", index, partition)
		}
		for _, topic := range partition.Topics {
			if !slices.Contains(requested.Topics, topic) {
				t.Fatalf("partition %d introduced topic %q", index, topic)
			}
		}
		for _, watch := range partition.Watches {
			if !slices.Contains(requested.Watches, watch) {
				t.Fatalf("partition %d introduced watch %+v", index, watch)
			}
		}
	}
	for _, topic := range requested.Topics {
		if !slices.ContainsFunc(partitions, func(partition Subscription) bool {
			return slices.Contains(partition.Topics, topic)
		}) {
			t.Fatalf("topic %q was dropped by %+v", topic, partitions)
		}
	}
	for _, watch := range requested.Watches {
		if !slices.ContainsFunc(partitions, func(partition Subscription) bool {
			return slices.Contains(partition.Watches, watch)
		}) {
			t.Fatalf("watch %+v was dropped by %+v", watch, partitions)
		}
		for _, topic := range workspaceObservedTopics(requested.Topics) {
			if !slices.ContainsFunc(partitions, func(partition Subscription) bool {
				return slices.Contains(partition.Watches, watch) &&
					slices.Contains(partition.Topics, FilesChanged) && slices.Contains(partition.Topics, topic)
			}) {
				t.Fatalf("workspace observation %s × %s was split: %+v", watch.ID, topic, partitions)
			}
		}
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

func TestPlanChangeIsAFirstClassInvalidation(t *testing.T) {
	t.Parallel()
	event := Event{Type: EventType(PlanChanged), Sequence: 1}
	if err := event.Validate(); err != nil {
		t.Fatal(err)
	}
}
