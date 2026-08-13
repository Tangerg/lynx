package operation

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/protocol"
)

type lifetimeService struct {
	Service
	streamStarted chan struct{}
}

func (s *lifetimeService) SubscribeRuntime(ctx context.Context, _ protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
	return &protocol.RuntimeSubscribeResponse{}, func(func(protocol.RuntimeEvent) bool) {
		close(s.streamStarted)
		<-ctx.Done()
	}, nil
}

func TestEndpointLifetimeEndsStreamsAndRejectsLaterCalls(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	service := &lifetimeService{streamStarted: make(chan struct{})}
	endpoint := New(service, Config{Lifetime: lifetime})
	result := endpoint.Invoke(t.Context(), "runtime.subscribe", protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	}, Options{})
	if result.Failure != nil || result.Events == nil {
		t.Fatalf("subscribe result = %+v", result)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range result.Events {
		}
	}()
	<-service.streamStarted
	stop()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Runtime lifetime cancellation did not end the stream")
	}

	result = endpoint.Invoke(t.Context(), "runtime.subscribe", protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	}, Options{})
	if !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("post-close failure = %v, want internal_error", result.Failure)
	}
}

func TestEndpointShutdownClaimsUnstartedStreamAndJoinsItsSource(t *testing.T) {
	lifetime, stop := context.WithCancel(context.Background())
	service := &lifetimeService{streamStarted: make(chan struct{})}
	endpoint := New(service, Config{Lifetime: lifetime})
	result := endpoint.Invoke(t.Context(), "runtime.subscribe", protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	}, Options{})
	if result.Failure != nil || result.Events == nil {
		t.Fatalf("subscribe result = %+v", result)
	}

	stop()
	select {
	case <-service.streamStarted:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not claim the unstarted stream source")
	}
	waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	if err := endpoint.AwaitShutdown(waitCtx); err != nil {
		t.Fatalf("AwaitShutdown: %v", err)
	}

	// The shutdown owner already exhausted this single-consumer sequence; a
	// caller that ranges its stale handle after Close observes a clean end.
	for range result.Events {
		t.Fatal("post-shutdown stream produced an event")
	}
}

type joiningStreamService struct {
	Service
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (s *joiningStreamService) SubscribeRuntime(
	ctx context.Context,
	_ protocol.RuntimeSubscribeRequest,
) (*protocol.RuntimeSubscribeResponse, iter.Seq[protocol.RuntimeEvent], error) {
	return &protocol.RuntimeSubscribeResponse{}, func(func(protocol.RuntimeEvent) bool) {
		close(s.started)
		<-ctx.Done()
		close(s.canceled)
		<-s.release
	}, nil
}

func TestEndpointShutdownWaitsForStartedStreamSourceToReturn(t *testing.T) {
	service := &joiningStreamService{
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
		release:  make(chan struct{}),
	}
	endpoint := New(service, Config{})
	result := endpoint.Invoke(t.Context(), "runtime.subscribe", protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	}, Options{})
	if result.Failure != nil || result.Events == nil {
		t.Fatalf("subscribe result = %+v", result)
	}
	streamDone := make(chan struct{})
	go func() {
		defer close(streamDone)
		for range result.Events {
		}
	}()
	select {
	case <-service.started:
	case <-time.After(time.Second):
		t.Fatal("stream source did not start")
	}

	endpoint.BeginShutdown()
	select {
	case <-service.canceled:
	case <-time.After(time.Second):
		t.Fatal("stream source did not observe shutdown")
	}
	shortCtx, cancelShort := context.WithTimeout(t.Context(), 20*time.Millisecond)
	if err := endpoint.AwaitShutdown(shortCtx); !errors.Is(err, context.DeadlineExceeded) {
		cancelShort()
		t.Fatalf("AwaitShutdown before source return = %v, want deadline exceeded", err)
	}
	cancelShort()

	close(service.release)
	select {
	case <-streamDone:
	case <-time.After(time.Second):
		t.Fatal("released stream source did not return")
	}
	waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
	defer cancelWait()
	if err := endpoint.AwaitShutdown(waitCtx); err != nil {
		t.Fatalf("AwaitShutdown after source return: %v", err)
	}
}
