package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/component/signal"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestScheduleFireNotificationProjectsToWorkspaceEvent(t *testing.T) {
	notifier := &signal.Signal[string]{}
	s := &Server{wsHub: newWorkspaceHub()}
	s.observeScheduleFires(notifier)
	events, unsubscribe := s.wsHub.subscribe()
	defer unsubscribe()

	notifier.Publish("sch_1")
	got := <-events
	if got.Type != protocol.WorkspaceEventSchedulesFired || got.ScheduleID != "sch_1" {
		t.Fatalf("workspace event = %+v", got)
	}
}
