package server

import (
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/component/signal"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
)

func TestScheduleFireNotificationProjectsToARuntimeSignal(t *testing.T) {
	notifier := &signal.Signal[string]{}
	s := &Server{wsHub: newWorkspaceHub()}
	s.observeScheduleFires(notifier)
	events, unsubscribe := s.wsHub.subscribe()
	defer unsubscribe()

	notifier.Publish("sch_1")
	got := <-events
	if got.Type != protocol.RuntimeSchedulesChanged || len(got.ScheduleIDs) != 1 || got.ScheduleIDs[0] != "sch_1" {
		t.Fatalf("runtime event = %+v", got)
	}
}
