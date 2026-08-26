package server

import (
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/protocol"
)

// TestPlanUpdateCarriesTheCompletePlan proves the first-class run event carries the
// same revisioned Plan shape that plan.get returns.
func TestPlanUpdateCarriesTheCompletePlan(t *testing.T) {
	t.Parallel()

	event := presentRunEvent(runs.PlanSnapshot{
		SessionID: "ses_1", Revision: 2, UpdatedAt: time.Unix(9, 0).UTC(),
		Steps: []plan.Step{{
			Description: "read the contract", Status: plan.StatusInProgress,
		}, {
			Description: "write the fixture", Status: plan.StatusPending,
		}},
	})

	if event.Type != protocol.StreamPlanUpdated || event.Plan == nil {
		t.Fatalf("event = %+v, want plan.updated with a Plan", event)
	}
	if event.Plan.SessionID != "ses_1" || event.Plan.Revision != 2 ||
		len(event.Plan.Steps) != 2 || event.Plan.Steps[0].Status != protocol.PlanStatusInProgress {
		t.Fatalf("Plan = %+v, want the complete revisioned replacement", event.Plan)
	}
}

// TestPlanQueryAnswersWithTheStreamsOwnSnapshot proves
// plan_revision_never_goes_backwards at the wire boundary.
//
// plan.get is the Plan's cold recovery source, so it is what a client calls when
// it missed the events — after a reload, a rollback, or a replay window it could not
// reach. The answer therefore has to be foldable by the SAME rule the stream is
// folded by: same shape, same key, and the store's own revision rather than a
// re-derived or zeroed one. A cold read that answered revision 0 would look older
// than every event the client already holds, and a monotonic fold would discard it —
// leaving the panel permanently stale in exactly the situation recovery exists for.
func TestPlanQueryAnswersWithTheStreamsOwnSnapshot(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	ses, err := insertSessionFixture(ctx, rt.sess, "recovering", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if saveTestPlanErr := saveTestPlan(ctx, rt.plan, ses.ID(), []plan.Step{{Description: "first", Status: plan.StatusCompleted}}); saveTestPlanErr != nil {
		t.Fatalf("seed plan: %v", saveTestPlanErr)
	}

	first, err := s.GetPlan(ctx, protocol.GetPlanRequest{SessionID: ses.ID()})
	if err != nil {
		t.Fatalf("plan.get: %v", err)
	}
	stored, err := rt.plan.State(ctx, ses.ID())
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if first.SessionID != ses.ID() {
		t.Fatalf("cold read = %+v, want the Plan for %s", first, ses.ID())
	}
	if first.Revision != stored.Revision() || first.Revision == 0 {
		t.Fatalf("cold read revision = %d, want the store's %d", first.Revision, stored.Revision())
	}
	if len(first.Steps) != 1 || first.Steps[0].Description != "first" {
		t.Fatalf("cold read list = %+v, want the stored list", first.Steps)
	}

	if saveTestPlanErr := saveTestPlan(ctx, rt.plan, ses.ID(), []plan.Step{{Description: "second", Status: plan.StatusInProgress}}); saveTestPlanErr != nil {
		t.Fatalf("advance plan: %v", saveTestPlanErr)
	}
	second, err := s.GetPlan(ctx, protocol.GetPlanRequest{SessionID: ses.ID()})
	if err != nil {
		t.Fatalf("plan.get again: %v", err)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("revision went from %d to %d; a later read must never answer older", first.Revision, second.Revision)
	}
}
