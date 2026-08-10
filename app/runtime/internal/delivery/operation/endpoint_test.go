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
