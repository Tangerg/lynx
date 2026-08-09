package server

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestSkillChangeBridgePublishesWorkspaceRefresh(t *testing.T) {
	s := &Server{workspaceHub: newWorkspaceHub()}
	notifier := new(testNotification[struct{}])
	s.observeSkillChanges(notifier)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, seq, err := s.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSkillsChanged},
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	events := drainSeq(ctx, seq)
	notifier.Publish(struct{}{})

	select {
	case event := <-events:
		if event.Type != protocol.RuntimeSkillsChanged {
			t.Fatalf("event = %+v, want skills.changed", event)
		}
	case <-time.After(time.Second):
		t.Fatal("skills refresh event not delivered")
	}
}
