package runtimeembedded

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/embedded"
	"github.com/Tangerg/lynx/app/runtime/protocol"

	"github.com/Tangerg/lynx/app/cli/internal/changefeed"
)

type changeBindingStub struct {
	request protocol.RuntimeSubscribeRequest
	events  iter.Seq2[protocol.RuntimeEvent, error]
	called  bool
}

func (stub *changeBindingStub) SubscribeRuntime(_ context.Context, request protocol.RuntimeSubscribeRequest, _ embedded.SubscriptionOptions) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
	stub.called, stub.request = true, request
	return &protocol.RuntimeSubscribeResponse{}, stub.events, nil
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
		supportedTopics: map[changefeed.Topic]struct{}{changefeed.FilesChanged: {}},
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

func TestChangefeedAdapterRefusesAnUnadvertisedTopic(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Runtime{changes: stub, supportedTopics: map[changefeed.Topic]struct{}{}}
	_, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []changefeed.Topic{changefeed.FilesChanged}})
	if err == nil {
		t.Fatal("unadvertised topic was accepted")
	}
	if stub.called {
		t.Fatal("runtime binding was called before capability validation")
	}
}

func TestChangefeedAdapterHonorsAdvertisedSubscriptionLimits(t *testing.T) {
	t.Parallel()
	stub := &changeBindingStub{}
	runtime := &Runtime{
		changes: stub, supportedTopics: map[changefeed.Topic]struct{}{changefeed.FilesChanged: {}},
		maxChangeWatches: 1,
	}
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
		changes: stub, supportedTopics: map[changefeed.Topic]struct{}{changefeed.FilesChanged: {}},
	}
	stream, err := runtime.Subscribe(t.Context(), changefeed.Subscription{Topics: []changefeed.Topic{changefeed.FilesChanged}})
	if err != nil {
		t.Fatal(err)
	}
	for _, eventErr := range stream {
		if eventErr == nil {
			t.Fatal("malformed wire event was accepted")
		}
		return
	}
	t.Fatal("malformed stream yielded no error")
}
