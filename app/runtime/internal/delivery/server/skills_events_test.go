package server

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

func TestSkillInvalidationPublishesWorkspaceRefresh(t *testing.T) {
	s := newWorkspaceServer(t.TempDir())
	s.workspaceHub = newWorkspaceHub()
	notifier := new(testNotification[invalidation.Notice])
	s.observeInvalidations(notifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)
	notifier.Publish(invalidation.Notice{Resource: invalidation.Skills})

	select {
	case event := <-events:
		if event.Type != protocol.RuntimeSkillsChanged {
			t.Fatalf("event = %+v, want skills.changed", event)
		}
	case <-time.After(time.Second):
		t.Fatal("skills refresh event not delivered")
	}
}
