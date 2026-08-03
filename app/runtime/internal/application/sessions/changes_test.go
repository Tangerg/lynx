package sessions

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/application/change"
)

// recorder collects notices in publish order.
type recorder struct{ notices []change.Notice }

func (r *recorder) publish(notice change.Notice) { r.notices = append(r.notices, notice) }

func (r *recorder) resources() []change.Resource {
	out := make([]change.Resource, 0, len(r.notices))
	for _, notice := range r.notices {
		out = append(out, notice.Resource)
	}
	return out
}

// TestDeleteSessionPublishesEveryProjectionItRemoved: a client told only "sessions
// changed" would keep a run list, a waiting set and a Plan belonging to a
// session that no longer exists. The delete cascade removes all of them, so it
// names all of them.
func TestDeleteSessionPublishesEveryProjectionItRemoved(t *testing.T) {
	stores := newMutationStores("")
	changes := &recorder{}
	coordinator := New(testDependencies(stores, Dependencies{
		Turns:   mutationTurns{operations: &stores.operations},
		Paths:   testCwdResolver{},
		Changed: changes.publish,
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	want := []change.Resource{change.Sessions, change.Runs, change.Interrupts, change.Goals, change.PlanState}
	if !slices.Equal(changes.resources(), want) {
		t.Fatalf("published = %v, want every projection the cascade removed (%v)", changes.resources(), want)
	}
	for _, notice := range changes.notices {
		if !slices.Equal(notice.SessionIDs, []string{"ses_1"}) {
			t.Fatalf("notice %d session ids = %v, want [ses_1]", notice.Resource, notice.SessionIDs)
		}
	}
}

// TestFailedDeleteSessionPublishesNothing: an invalidation for a transaction that
// rolled back sends every listener to re-read state that never changed — and, for a
// delete, to conclude a session is gone while it is still there.
func TestFailedDeleteSessionPublishesNothing(t *testing.T) {
	stores := newMutationStores("apply.delete")
	changes := &recorder{}
	coordinator := New(testDependencies(stores, Dependencies{
		Turns:   mutationTurns{operations: &stores.operations},
		Paths:   testCwdResolver{},
		Changed: changes.publish,
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if len(changes.notices) != 0 {
		t.Fatalf("published %v after a failed commit, want nothing", changes.resources())
	}
}
