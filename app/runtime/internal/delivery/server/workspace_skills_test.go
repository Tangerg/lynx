package server

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/component/signal"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestSkillChangeBridgePublishesWorkspaceRefresh(t *testing.T) {
	s := &Server{wsHub: newWorkspaceHub()}
	notifier := new(signal.Signal[struct{}])
	s.observeSkillChanges(notifier)

	_, events, err := s.WorkspaceSubscribe(context.Background(), protocol.WorkspaceSubscribeRequest{})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	notifier.Publish(struct{}{})

	select {
	case event := <-events:
		if event.Type != protocol.WorkspaceEventSkillsChanged {
			t.Fatalf("event = %+v, want skills.changed", event)
		}
	case <-time.After(time.Second):
		t.Fatal("skills refresh event not delivered")
	}
}
