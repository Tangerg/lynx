package server

import (
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/scope/app/runtime/protocol"
)

func TestScheduleInvalidationProjectsToARuntimeSignal(t *testing.T) {
	notifier := &testNotification[invalidation.Notice]{}
	s := &Server{workspaceHub: newWorkspaceHub()}
	s.observeInvalidations(notifier.Observe)
	events, unsubscribe := s.workspaceHub.subscribe()
	defer unsubscribe()

	notifier.Publish(invalidation.ForSchedules("sch_1"))
	got := <-events
	if got.Type != protocol.RuntimeSchedulesChanged || len(got.ScheduleIDs) != 1 || got.ScheduleIDs[0] != "sch_1" {
		t.Fatalf("runtime event = %+v", got)
	}
}
