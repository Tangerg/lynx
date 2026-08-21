package server

import (
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// TestEveryInvalidationResourceIsPublishable: the application's resource set and the wire
// topics are two spellings of one vocabulary, and the projection between them is
// hand-written. A resource with no mapping would be a committed invalidation no client is
// ever told about — silent, and invisible to every other test.
func TestEveryInvalidationResourceIsPublishable(t *testing.T) {
	for _, resource := range []invalidation.Resource{
		invalidation.Sessions,
		invalidation.Runs,
		invalidation.Interrupts,
		invalidation.Goals,
		invalidation.PlanState,
		invalidation.Schedules,
		invalidation.Knowledge,
		invalidation.Hooks,
		invalidation.Skills,
		invalidation.MCP,
		invalidation.Models,
		invalidation.Approvals,
		invalidation.AgentMemory,
		invalidation.Codebase,
	} {
		notice := invalidation.Notice{Resource: resource, SessionIDs: []string{"ses_1"}}
		if resource == invalidation.Schedules {
			notice = invalidation.ForSchedules("sch_1")
		}
		ev, ok := runtimeEventFor(notice)
		if !ok {
			t.Fatalf("resource %d has no runtime event", resource)
		}
		if !slices.Contains(protocol.RuntimeTopics(), protocol.RuntimeTopic(ev.Type)) {
			t.Fatalf("resource %d maps to %q, which is not a subscribable topic", resource, ev.Type)
		}
		sessionScoped := resource == invalidation.Sessions || resource == invalidation.Runs ||
			resource == invalidation.Interrupts || resource == invalidation.Goals ||
			resource == invalidation.PlanState
		if sessionScoped && !slices.Equal(ev.SessionIDs, []string{"ses_1"}) {
			t.Fatalf("resource %d dropped the session scope: %+v", resource, ev.SessionIDs)
		}
		if resource == invalidation.Schedules && !slices.Equal(ev.ScheduleIDs, []string{"sch_1"}) {
			t.Fatalf("schedule invalidation dropped its scope: %+v", ev.ScheduleIDs)
		}
	}
}

// TestPlanChangeKeepsSessionScope proves plan.changed names the product resource
// directly and carries only the Session identity needed by plan.get.
//
// It is the wire half of session_plan_is_owned_by_its_session and of
// committed_plan_change_reaches_other_windows: the store keeps one value per
// session, and this is where that scope survives being published.
func TestPlanChangeKeepsSessionScope(t *testing.T) {
	ev, ok := runtimeEventFor(invalidation.Notice{
		Resource: invalidation.PlanState, SessionIDs: []string{"ses_1"}, RunIDs: []string{"run_1"},
	})
	if !ok {
		t.Fatal("Plan has no runtime event")
	}
	if ev.Type != protocol.RuntimePlanChanged {
		t.Fatalf("event = %s, want plan.changed", ev.Type)
	}
	if !slices.Equal(ev.SessionIDs, []string{"ses_1"}) || len(ev.RunIDs) != 0 {
		t.Fatalf("scope = sessions %v runs %v, want only ses_1", ev.SessionIDs, ev.RunIDs)
	}
}
