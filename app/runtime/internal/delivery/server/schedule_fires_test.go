package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestScheduleFireNotificationProjectsToARuntimeSignal(t *testing.T) {
	notifier := &testNotification[string]{}
	s := &Server{workspaceHub: newWorkspaceHub()}
	s.observeScheduleFires(notifier)
	events, unsubscribe := s.workspaceHub.subscribe()
	defer unsubscribe()

	notifier.Publish("sch_1")
	got := <-events
	if got.Type != protocol.RuntimeSchedulesChanged || len(got.ScheduleIDs) != 1 || got.ScheduleIDs[0] != "sch_1" {
		t.Fatalf("runtime event = %+v", got)
	}
}
