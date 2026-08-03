package server

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/runs"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/lynx/app/runtime/internal/delivery/protocol"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

// TestStateSnapshotCarriesItsDeclaredPlanPayload is the shape fixture for the
// "plan" state key (contract §11.4 gate 14).
//
// The registry publishes the key's payload from StateKeySpec.PayloadType and the
// presenter builds the value; nothing connects the two, so a client reading the
// published shape and a runtime emitting a different one would both look correct.
//
// So the produced value is put on the wire and read back through the DECLARED type,
// and re-encoded to compare: a field the declared type cannot represent disappears on
// the way back, and a field it requires that the presenter never sets shows up.
func TestStateSnapshotCarriesItsDeclaredPlanPayload(t *testing.T) {
	t.Parallel()

	const key = "plan"
	declared := declaredStatePayload(t, key)

	event := presentRunEvent(runs.StateSnapshot{
		SessionID: "ses_1", Revision: 2, UpdatedAt: time.Unix(9, 0).UTC(),
		Plan: []runs.PlanSnapshot{{
			ID: "plan_1", Description: "read the contract", Status: plan.StatusInProgress,
		}, {
			ID: "plan_2", Description: "write the fixture", Status: plan.StatusPending,
		}},
	})

	if event.State == nil || event.State.Type != protocol.StatePlan {
		t.Fatalf("the snapshot carries %+v, not the %q key", event.State, key)
	}
	onTheWire, err := json.Marshal(event.State)
	if err != nil {
		t.Fatalf("marshal the produced payload: %v", err)
	}

	readBack := reflect.New(declared)
	if err := json.Unmarshal(onTheWire, readBack.Interface()); err != nil {
		t.Fatalf("the produced %q payload does not read back as %s: %v", key, declared, err)
	}
	reencoded, err := json.Marshal(readBack.Elem().Interface())
	if err != nil {
		t.Fatalf("re-marshal through the declared type: %v", err)
	}
	if string(reencoded) != string(onTheWire) {
		t.Errorf("the produced %q payload and its declared shape disagree\n produced:  %s\n declared:  %s",
			key, onTheWire, reencoded)
	}
}

func declaredStatePayload(t *testing.T, key string) reflect.Type {
	t.Helper()

	for _, spec := range dispatch.WireShapes().StateKeys() {
		if spec.Key == key {
			return spec.PayloadType
		}
	}
	t.Fatalf("no state key %q is registered", key)
	return nil
}

// TestPlanQueryAnswersWithTheStreamsOwnSnapshot proves
// state_revision_never_goes_backwards at the wire boundary.
//
// plan.get is the key's declared recovery source, so it is what a client calls when
// it missed the events — after a reload, a rollback, or a replay window it could not
// reach. The answer therefore has to be foldable by the SAME rule the stream is
// folded by: same shape, same key, and the store's own revision rather than a
// re-derived or zeroed one. A cold read that answered revision 0 would look older
// than every event the client already holds, and a monotonic fold would discard it —
// leaving the panel permanently stale in exactly the situation recovery exists for.
func TestPlanQueryAnswersWithTheStreamsOwnSnapshot(t *testing.T) {
	s, rt := rollbackHarness(t)
	ctx := t.Context()
	ses, err := rt.sess.Create(ctx, "recovering", t.TempDir())
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := rt.plan.Replace(ctx, ses.ID, []plan.Step{{Description: "first", Status: plan.StatusCompleted}}); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	first, err := s.GetPlan(ctx, protocol.GetPlanRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("plan.get: %v", err)
	}
	stored, err := rt.plan.State(ctx, ses.ID)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	if first.Type != protocol.StatePlan || first.SessionID != ses.ID {
		t.Fatalf("cold read = %+v, want the plan key for %s", first, ses.ID)
	}
	if first.Revision != stored.Revision || first.Revision == 0 {
		t.Fatalf("cold read revision = %d, want the store's %d", first.Revision, stored.Revision)
	}
	if len(first.Plan) != 1 || first.Plan[0].Description != "first" {
		t.Fatalf("cold read list = %+v, want the stored list", first.Plan)
	}

	if err := rt.plan.Replace(ctx, ses.ID, []plan.Step{{Description: "second", Status: plan.StatusInProgress}}); err != nil {
		t.Fatalf("advance plan: %v", err)
	}
	second, err := s.GetPlan(ctx, protocol.GetPlanRequest{SessionID: ses.ID})
	if err != nil {
		t.Fatalf("plan.get again: %v", err)
	}
	if second.Revision <= first.Revision {
		t.Fatalf("revision went from %d to %d; a later read must never answer older", first.Revision, second.Revision)
	}
}
