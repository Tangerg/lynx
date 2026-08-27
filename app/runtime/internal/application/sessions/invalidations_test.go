package sessions

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/scope/app/runtime/internal/application/invalidation"
)

// invalidationRecorder collects notices in publish order.
type invalidationRecorder struct{ notices []invalidation.Notice }

func (i *invalidationRecorder) publish(notice invalidation.Notice) {
	i.notices = append(i.notices, notice)
}

func (i *invalidationRecorder) resources() []invalidation.Resource {
	out := make([]invalidation.Resource, 0, len(i.notices))
	for _, notice := range i.notices {
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
	invalidations := &invalidationRecorder{}
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
		Invalidations:     invalidations.publish,
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	want := []invalidation.Resource{invalidation.Sessions, invalidation.Runs, invalidation.Interrupts, invalidation.Goals, invalidation.PlanState}
	if !slices.Equal(invalidations.resources(), want) {
		t.Fatalf("published = %v, want every projection the cascade removed (%v)", invalidations.resources(), want)
	}
	for _, notice := range invalidations.notices {
		if !slices.Equal(notice.SessionIDs, []string{"ses_1"}) {
			t.Fatalf("notice %q session ids = %v, want [ses_1]", notice.Resource, notice.SessionIDs)
		}
	}
}

// TestFailedDeleteSessionPublishesNothing: an invalidation for a transaction that
// rolled back sends every listener to re-read state that never changed — and, for a
// delete, to conclude a session is gone while it is still there.
func TestFailedDeleteSessionPublishesNothing(t *testing.T) {
	stores := newMutationStores("apply.delete")
	invalidations := &invalidationRecorder{}
	coordinator := mustNewCoordinator(testDependencies(stores, Dependencies{
		ExecutionReleaser: mutationExecutions{operations: &stores.operations},
		Paths:             testWorkspaceResolver{},
		Invalidations:     invalidations.publish,
	}))

	if err := coordinator.DeleteSession(t.Context(), "ses_1"); !errors.Is(err, errMutationStage) {
		t.Fatalf("DeleteSession error = %v, want %v", err, errMutationStage)
	}
	if len(invalidations.notices) != 0 {
		t.Fatalf("published %v after a failed commit, want nothing", invalidations.resources())
	}
}
