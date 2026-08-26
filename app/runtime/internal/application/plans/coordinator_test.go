package plans

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/application/invalidation"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
)

type fakeStore struct {
	state            plan.State
	expectedRevision uint64
	saved            plan.State
	readErr          error
	saveErr          error
}

func (f *fakeStore) State(context.Context, string) (plan.State, error) { return f.state, f.readErr }
func (f *fakeStore) Save(_ context.Context, _ string, expected uint64, replacement plan.State) error {
	f.expectedRevision = expected
	f.saved = replacement
	return f.saveErr
}

// TestCommittedPlanChangeReachesOtherWindows proves
// committed_plan_change_reaches_other_windows at its mutation owner: only a
// successful CAS publishes a session-scoped Plan invalidation.
func TestCommittedPlanChangeReachesOtherWindows(t *testing.T) {
	now := time.Date(2026, 8, 10, 2, 3, 4, 0, time.UTC)
	current, err := plan.Restore(plan.Snapshot{Revision: 3, UpdatedAt: now.Add(-time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{state: current}
	var notices []invalidation.Notice
	coordinator := New(Dependencies{
		Store: store, Now: func() time.Time { return now },
		Invalidations: func(notice invalidation.Notice) { notices = append(notices, notice) },
	})
	got, err := coordinator.Replace(t.Context(), "ses_1", []plan.Step{{Description: "ship", Status: plan.StatusInProgress}})
	if err != nil {
		t.Fatalf("Replace: %v", err)
	}
	if store.expectedRevision != 3 || store.saved.Revision() != 4 || got.Revision() != 4 || !got.UpdatedAt().Equal(now) {
		t.Fatalf("saved = %+v expected=%d got=%+v", store.saved.Snapshot(), store.expectedRevision, got.Snapshot())
	}
	if len(notices) != 1 || notices[0].Resource != invalidation.PlanState || len(notices[0].SessionIDs) != 1 || notices[0].SessionIDs[0] != "ses_1" {
		t.Fatalf("notices = %+v, want the committed Plan state", notices)
	}
}

func TestPrepareReplacementDoesNotWrite(t *testing.T) {
	store := &fakeStore{}
	coordinator := New(Dependencies{Store: store, Now: func() time.Time { return time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC) }})
	replacement, err := coordinator.PrepareReplacement(t.Context(), "ses_1", nil)
	if err != nil {
		t.Fatalf("PrepareReplacement: %v", err)
	}
	if replacement.ExpectedRevision() != 0 || replacement.State().Revision() != 1 || store.saved.Revision() != 0 {
		t.Fatalf("replacement = expected %d state %+v; store was %+v", replacement.ExpectedRevision(), replacement.State().Snapshot(), store.saved.Snapshot())
	}
}

func TestReplacePropagatesRevisionConflict(t *testing.T) {
	store := &fakeStore{saveErr: plan.ErrRevisionConflict}
	var published bool
	coordinator := New(Dependencies{
		Store: store, Now: time.Now,
		Invalidations: func(invalidation.Notice) { published = true },
	})
	_, err := coordinator.Replace(t.Context(), "ses_1", nil)
	if !errors.Is(err, plan.ErrRevisionConflict) {
		t.Fatalf("Replace error = %v, want ErrRevisionConflict", err)
	}
	if published {
		t.Fatal("failed replacement published a Plan change")
	}
}

func TestStateRejectsInvalidSessionIdentityBeforePersistence(t *testing.T) {
	store := &fakeStore{}
	coordinator := New(Dependencies{Store: store, Now: time.Now})
	for _, sessionID := range []string{"", " ses_1", "ses_1 "} {
		if _, err := coordinator.State(t.Context(), sessionID); err == nil {
			t.Errorf("State(%q) succeeded", sessionID)
		}
	}
}
