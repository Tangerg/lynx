package runtimeembedded

import (
	"context"
	"errors"
	"iter"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
	"github.com/Tangerg/lynx/app/cli/internal/runtimeprofile"
)

type changeBindingStub struct {
	request protocol.RuntimeSubscribeRequest
	events  iter.Seq2[protocol.RuntimeEvent, error]
	called  bool
}

func (c *changeBindingStub) SubscribeRuntime(_ context.Context, request protocol.RuntimeSubscribeRequest, _ embedded.SubscriptionOptions) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	c.called, c.request = true, request
	return &protocol.RuntimeSubscribeResponse{}, c.events, nil
}

func TestChangefeedAdapterNegotiatesAndProjectsRuntimeEvents(t *testing.T) {
	t.Parallel()
	workspaceRef := protocol.WorkspaceRef{Path: "/workspace"}
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{
			Type: protocol.RuntimeFilesChanged, Sequence: 1, WatchID: "active",
			Workspace: &workspaceRef, Paths: []string{"main.go"},
		}, nil)
	}}
	runtime := &Runtime{
		changes: stub, meta: requestMeta("test"),
		profile: changefeedProfile(changefeed.FilesChanged),
	}
	runtime.profile.Features = map[runtimeprofile.FeatureName]runtimeprofile.Feature{
		runtimeprofile.FeatureFileWatch: {Enabled: true},
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics:  []changefeed.Topic{changefeed.FilesChanged},
		Watches: []changefeed.Watch{{ID: "active", Workspace: "/workspace"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var events []changefeed.Event
	for event, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Workspace != "/workspace" || events[0].Paths[0] != "main.go" {
		t.Fatalf("events = %+v", events)
	}
	if !stub.called || len(stub.request.Watches) != 1 || stub.request.Watches[0].WatchID != "active" {
		t.Fatalf("request = %+v", stub.request)
	}
}

func TestChangefeedAdapterProjectsBroadFileInvalidations(t *testing.T) {
	t.Parallel()
	workspaceRef := protocol.WorkspaceRef{Path: "/workspace"}
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{
			Type: protocol.RuntimeFilesChanged, Sequence: 1,
			Workspace: &workspaceRef, Paths: []string{"main.go"},
		}, nil)
	}}
	runtime := &Runtime{
		changes: stub, meta: requestMeta("test"),
		profile: changefeedProfile(changefeed.FilesChanged),
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics: []changefeed.Topic{changefeed.FilesChanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	for event, eventErr := range stream {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if event.WatchID != "" || event.Workspace != "/workspace" || !slices.Equal(event.Paths, []string{"main.go"}) {
			t.Fatalf("broad file event = %+v", event)
		}
		return
	}
	t.Fatal("broad file stream yielded no event")
}

func TestChangefeedAdapterRejectsEventsOutsideTheSubscription(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		subscription changefeed.Subscription
		event        protocol.RuntimeEvent
	}{
		{
			name:         "topic",
			subscription: changefeed.Subscription{Topics: []changefeed.Topic{changefeed.SessionsChanged}},
			event:        protocol.RuntimeEvent{Type: protocol.RuntimeRunsChanged, Sequence: 1},
		},
		{
			name: "watch",
			subscription: changefeed.Subscription{
				Topics:  []changefeed.Topic{changefeed.FilesChanged},
				Watches: []changefeed.Watch{{ID: "active", Workspace: "/workspace"}},
			},
			event: protocol.RuntimeEvent{
				Type: protocol.RuntimeFilesChanged, Sequence: 1,
				WatchID: "foreign", Paths: []string{"main.go"},
			},
		},
		{
			name:         "resync",
			subscription: changefeed.Subscription{Topics: []changefeed.Topic{changefeed.SessionsChanged}},
			event: protocol.RuntimeEvent{
				Type: protocol.RuntimeResync, Sequence: 1,
				Topics: []protocol.RuntimeTopic{protocol.TopicRunsChanged},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
				yield(test.event, nil)
			}}
			runtime := &Runtime{
				changes: stub, meta: requestMeta("test"),
				profile: changefeedProfile(changefeed.FilesChanged, changefeed.SessionsChanged, changefeed.RunsChanged),
			}
			if len(test.subscription.Watches) != 0 {
				runtime.profile.Features = map[runtimeprofile.FeatureName]runtimeprofile.Feature{
					runtimeprofile.FeatureFileWatch: {Enabled: true},
				}
			}
			stream, err := runtime.Subscribe(t.Context(), test.subscription)
			if err != nil {
				t.Fatal(err)
			}
			for _, eventErr := range stream {
				requireRuntimeContractViolation(t, eventErr)
				return
			}
			t.Fatal("out-of-scope stream yielded no error")
		})
	}
}

func TestChangefeedAdapterRefusesAnUnadvertisedTopic(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Runtime{changes: stub}
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []changefeed.Topic{changefeed.FilesChanged}})
	if err == nil {
		t.Fatal("unadvertised topic was accepted")
	}
	if stub.called {
		t.Fatal("runtime binding was called before capability validation")
	}
}

func TestChangefeedAdapterRejectsWatchesWithoutFileWatchCapability(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Runtime{changes: stub, profile: changefeedProfile(changefeed.FilesChanged)}
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics:  []changefeed.Topic{changefeed.FilesChanged},
		Watches: []changefeed.Watch{{ID: "active", Workspace: "/workspace"}},
	})
	if err == nil || !errors.Is(err, agent.ErrIncompatibleRuntime) {
		t.Fatalf("Subscribe error = %v, want ErrIncompatibleRuntime", err)
	}
	if stub.called {
		t.Fatal("workspace watch reached the binding without fileWatch capability")
	}
}

func TestChangefeedAdapterHonorsAdvertisedSubscriptionLimits(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Runtime{
		changes: stub, profile: changefeedProfile(changefeed.FilesChanged),
	}
	runtime.profile.Limits.RuntimeSubscription.MaxWatches = 1
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{
		Topics:  []changefeed.Topic{changefeed.FilesChanged},
		Watches: []changefeed.Watch{{ID: "one", Workspace: "/one"}, {ID: "two", Workspace: "/two"}},
	})
	if err == nil {
		t.Fatal("subscription above the advertised watch limit was accepted")
	}
	if stub.called {
		t.Fatal("binding was called before subscription limit validation")
	}
}

func TestChangefeedAdapterRejectsMalformedWireEvent(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{Type: protocol.RuntimeFilesChanged}, nil)
	}}
	runtime := &Runtime{
		changes: stub, profile: changefeedProfile(changefeed.FilesChanged),
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []changefeed.Topic{changefeed.FilesChanged}})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventErr := range stream {
		if eventErr == nil {
			t.Fatal("malformed wire event was accepted")
		}
		requireRuntimeContractViolation(t, eventErr)
		return
	}
	t.Fatal("malformed stream yielded no error")
}

func TestChangefeedAdapterProjectsRuntimeResourceInvalidations(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		topic changefeed.Topic
		event protocol.RuntimeEventType
	}{
		{name: "models", topic: changefeed.ModelsChanged, event: protocol.RuntimeModelsChanged},
		{name: "approvals", topic: changefeed.ApprovalsChanged, event: protocol.RuntimeApprovalsChanged},
		{name: "agent memory", topic: changefeed.AgentMemoryChanged, event: protocol.RuntimeAgentMemoryChanged},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
				yield(protocol.RuntimeEvent{Type: test.event, Sequence: 1}, nil)
			}}
			runtime := &Runtime{changes: stub, profile: changefeedProfile(test.topic)}
			stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []changefeed.Topic{test.topic}})
			if err != nil {
				t.Fatal(err)
			}
			for event, eventErr := range stream {
				if eventErr != nil {
					t.Fatal(eventErr)
				}
				if event.Type != changefeed.EventType(test.topic) || event.Sequence != 1 {
					t.Fatalf("projected event = %+v, want %s sequence 1", event, test.topic)
				}
				return
			}
			t.Fatal("runtime resource stream yielded no event")
		})
	}
}

func TestChangefeedAdapterRejectsAnIncompleteRuntimeStream(t *testing.T) {
	t.Parallel()
	runtime := &Runtime{
		changes: &changeBindingStub{}, profile: changefeedProfile(changefeed.FilesChanged),
	}
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []changefeed.Topic{changefeed.FilesChanged}})
	requireRuntimeContractViolation(t, err)
}

func TestChangefeedAdapterProjectsPlanInvalidation(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{events: func(yield func(protocol.RuntimeEvent, error) bool) {
		yield(protocol.RuntimeEvent{
			Type: protocol.RuntimePlanChanged, Sequence: 1,
			SessionIDs: []string{"ses_1"},
		}, nil)
	}}
	runtime := &Runtime{
		changes: stub, profile: changefeedProfile(changefeed.PlanChanged),
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []changefeed.Topic{changefeed.PlanChanged}})
	if err != nil {
		t.Fatal(err)
	}
	for event, eventErr := range stream {
		if eventErr != nil {
			t.Fatal(eventErr)
		}
		if len(event.SessionIDs) != 1 || event.SessionIDs[0] != "ses_1" {
			t.Fatalf("plan invalidation sessions = %v", event.SessionIDs)
		}
		return
	}
	t.Fatal("plan change stream yielded no event")
}

func changefeedProfile(topics ...changefeed.Topic) runtimeprofile.Profile {
	profile := runtimeprofile.Profile{
		Limits: runtimeprofile.Limits{RuntimeSubscription: runtimeprofile.SubscriptionLimits{MaxTopics: 32, MaxWatches: 32}},
	}
	for _, topic := range topics {
		profile.RuntimeTopics = append(profile.RuntimeTopics, string(topic))
	}
	return profile
}
