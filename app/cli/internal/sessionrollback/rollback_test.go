package sessionrollback

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/cli/internal/agent"
	"github.com/Tangerg/lynx/app/cli/internal/agent/mock"
	"github.com/Tangerg/lynx/app/cli/internal/mutation"
	"github.com/Tangerg/lynx/app/cli/internal/retry"
	"github.com/Tangerg/lynx/app/cli/internal/workbench"
)

type recordingRuntime struct {
	*mock.Runtime

	calls     int
	request   agent.RollbackSession
	reject    error
	afterCall func()
}

func (r *recordingRuntime) RollbackSession(
	ctx context.Context,
	request agent.RollbackSession,
) (agent.RollbackResult, error) {
	r.calls++
	r.request = request
	reject := r.reject
	if r.afterCall != nil {
		r.afterCall()
	}
	if reject != nil {
		return agent.RollbackResult{}, reject
	}
	return r.Runtime.RollbackSession(ctx, request)
}

func TestFileRollbackStopsRetryingWhenReplayExpires(t *testing.T) {
	underlying := mock.New()
	snapshot, err := underlying.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewSession(snapshot, agent.RollbackSession{
		SessionID: snapshot.Session.ID, ToRunID: snapshot.Runs[0].ID, Scope: agent.RestoreFiles,
	})
	if err != nil {
		t.Fatal(err)
	}
	stagedAt := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	window := ReplayWindow{Namespace: "idp_original", Retention: time.Minute}
	pending := preview.journal(
		agent.CommandID("cli_99999999999999999999999999999999"), window, stagedAt,
	)
	now := pending.ReplayUntil.Add(-time.Nanosecond)
	window.Now = func() time.Time { return now }
	runtime := &recordingRuntime{Runtime: underlying, reject: agent.ErrDisconnected}
	runtime.afterCall = func() {
		now = pending.ReplayUntil
		runtime.reject = nil
	}
	result, err := Settle(t.Context(), runtime, pending, window, retry.Backoff{})
	if result.Outcome != mutation.Unknown || !errors.Is(err, mutation.ErrReplayGuaranteeUnavailable) {
		t.Fatalf("settlement = outcome %v, error %v", result.Outcome, err)
	}
	if runtime.calls != 1 {
		t.Fatalf("expired rollback reached runtime %d times", runtime.calls)
	}
}

func rollbackFixture(t *testing.T, request agent.RollbackSession) (*mock.Runtime, Preview) {
	t.Helper()
	runtime := mock.New()
	snapshot, err := runtime.GetSession(t.Context(), request.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := PreviewSession(snapshot, request)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, preview
}

func TestRecoverConfirmsAnAlreadyAppliedRollbackWithoutReplay(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	window := ReplayWindow{Now: func() time.Time {
		return time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	}}
	pending := preview.journal(
		agent.CommandID("cli_11111111111111111111111111111111"), window, window.now(),
	)
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if stageSessionRollbackErr := store.StageSessionRollback(pending); stageSessionRollbackErr != nil {
		t.Fatal(stageSessionRollbackErr)
	}
	if _, rollbackSessionErr := underlying.RollbackSession(t.Context(), pending.Request()); rollbackSessionErr != nil {
		t.Fatal(rollbackSessionErr)
	}
	runtime := &recordingRuntime{Runtime: underlying}
	if recoverErr := Recover(t.Context(), runtime, store, window, retry.Backoff{}); recoverErr != nil {
		t.Fatal(recoverErr)
	}
	if runtime.calls != 0 {
		t.Fatalf("already-applied rollback was replayed %d times", runtime.calls)
	}
	confirmed, exists := store.PendingSessionRollback(pending.SessionID)
	if !exists || confirmed.Phase != workbench.SessionRollbackConfirmed {
		t.Fatalf("confirmed rollback = %+v, present %t", confirmed, exists)
	}
	recovery, found, err := store.ConsumeConfirmedSessionRollback(pending.SessionID)
	if err != nil || !found || recovery.Draft.Text != "Why is the cache expiry test flaky?" {
		t.Fatalf("rollback recovery = %+v, present %t, err %v", recovery, found, err)
	}
}

func TestPreviewKeepsTheBoundaryRootDescendants(t *testing.T) {
	runtime := mock.New()
	snapshot, err := runtime.GetSession(t.Context(), "ses_demo_1")
	if err != nil {
		t.Fatal(err)
	}
	root := snapshot.Runs[0]
	child := root.Clone()
	child.ID = "run_child"
	child.Lineage = agent.RunLineage{
		SpawnedByBlockID: "item_delegate", ParentRunID: root.ID, RootRunID: root.ID,
	}
	child.CreatedAt = root.CreatedAt.Add(time.Millisecond)
	later := root.Clone()
	later.ID = "run_later"
	later.CreatedAt = root.CreatedAt.Add(2 * time.Millisecond)
	snapshot.Runs = []agent.Run{root, child, later}
	if validateErr := snapshot.Validate(); validateErr != nil {
		t.Fatal(validateErr)
	}
	preview, err := PreviewSession(snapshot, agent.RollbackSession{
		SessionID: snapshot.Session.ID, ToRunID: root.ID, Scope: agent.RestoreHistory,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(preview.afterRunIDs, []string{root.ID, child.ID}) || preview.DroppedCount() != 1 {
		t.Fatalf("rollback projection after = %v, dropped %d", preview.afterRunIDs, preview.DroppedCount())
	}
	applied := snapshot
	applied.Session.Revision++
	applied.Runs = slices.Clone(snapshot.Runs[:2])
	if err := preview.ValidateApplied(applied); err != nil {
		t.Fatalf("root subtree rollback outcome: %v", err)
	}
}

func TestRecoverReplaysAPreparedHistoryRollbackWithItsStableIdentity(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	window := ReplayWindow{Now: func() time.Time {
		return time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	}}
	pending := preview.journal(
		agent.CommandID("cli_22222222222222222222222222222222"), window, window.now(),
	)
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StageSessionRollback(pending); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{Runtime: underlying}
	if err := Recover(t.Context(), runtime, store, window, retry.Backoff{}); err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 1 || runtime.request != pending.Request() {
		t.Fatalf("rollback replay = %+v after %d calls", runtime.request, runtime.calls)
	}
	confirmed, exists := store.PendingSessionRollback(pending.SessionID)
	if !exists || confirmed.Phase != workbench.SessionRollbackConfirmed {
		t.Fatalf("confirmed rollback = %+v, present %t", confirmed, exists)
	}
}

func TestRecoverRefusesUnprovenFileRollbackReplay(t *testing.T) {
	for _, test := range []struct {
		name    string
		current ReplayWindow
	}{
		{
			name: "another runtime namespace",
			current: ReplayWindow{Namespace: "idp_other", Retention: time.Minute, Now: func() time.Time {
				return time.Date(2026, 8, 13, 10, 0, 30, 0, time.UTC)
			}},
		},
		{
			name: "replay deadline reached",
			current: ReplayWindow{Namespace: "idp_original", Retention: time.Minute, Now: func() time.Time {
				return time.Date(2026, 8, 13, 10, 1, 0, 0, time.UTC)
			}},
		},
		{
			name: "expired replay window",
			current: ReplayWindow{Namespace: "idp_original", Retention: time.Minute, Now: func() time.Time {
				return time.Date(2026, 8, 13, 10, 2, 0, 0, time.UTC)
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			underlying := mock.New()
			snapshot, err := underlying.GetSession(t.Context(), "ses_demo_1")
			if err != nil {
				t.Fatal(err)
			}
			preview, err := PreviewSession(snapshot, agent.RollbackSession{
				SessionID: snapshot.Session.ID, ToRunID: snapshot.Runs[0].ID, Scope: agent.RestoreFiles,
			})
			if err != nil {
				t.Fatal(err)
			}
			stagedAt := time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
			original := ReplayWindow{Namespace: "idp_original", Retention: time.Minute}
			pending := preview.journal(
				agent.CommandID("cli_33333333333333333333333333333333"), original, stagedAt,
			)
			store, err := workbench.Open(t.TempDir(), workbench.Config{})
			if err != nil {
				t.Fatal(err)
			}
			if stageSessionRollbackErr := store.StageSessionRollback(pending); stageSessionRollbackErr != nil {
				t.Fatal(stageSessionRollbackErr)
			}
			runtime := &recordingRuntime{Runtime: underlying}
			err = Recover(t.Context(), runtime, store, test.current, retry.Backoff{})
			if err == nil || !strings.Contains(err.Error(), "replay guarantee") {
				t.Fatalf("file rollback recovery error = %v", err)
			}
			if runtime.calls != 0 {
				t.Fatalf("unsafe file rollback was replayed %d times", runtime.calls)
			}
			stored, exists := store.PendingSessionRollback(pending.SessionID)
			if !exists || stored.Phase != workbench.SessionRollbackPrepared {
				t.Fatalf("preserved file rollback = %+v, present %t", stored, exists)
			}
		})
	}
}

func TestRecoverRetiresADefinitivelyRejectedHistoryRollback(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	window := ReplayWindow{Now: func() time.Time {
		return time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	}}
	pending := preview.journal(
		agent.CommandID("cli_44444444444444444444444444444444"), window, window.now(),
	)
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StageSessionRollback(pending); err != nil {
		t.Fatal(err)
	}
	runtime := &recordingRuntime{Runtime: underlying, reject: agent.ErrSessionBusy}
	if err := Recover(t.Context(), runtime, store, window, retry.Backoff{}); err != nil {
		t.Fatal(err)
	}
	if runtime.calls != 1 || !errors.Is(runtime.reject, agent.ErrSessionBusy) {
		t.Fatalf("rejected rollback calls = %d, error %v", runtime.calls, runtime.reject)
	}
	if pending := store.PendingSessionRollbacks(); len(pending) != 0 {
		t.Fatalf("rejected rollback journals = %+v", pending)
	}
}

func TestRecoverPreservesHistoryRollbackRejectedByAnotherRuntimeStore(t *testing.T) {
	underlying, preview := rollbackFixture(t, agent.RollbackSession{
		SessionID: "ses_demo_1", Scope: agent.RestoreHistory,
	})
	window := ReplayWindow{Now: func() time.Time {
		return time.Date(2026, 8, 13, 10, 0, 0, 0, time.UTC)
	}}
	pending := preview.journal(
		agent.CommandID("cli_55555555555555555555555555555555"), window, window.now(),
	)
	store, err := workbench.Open(t.TempDir(), workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if stageSessionRollbackErr := store.StageSessionRollback(pending); stageSessionRollbackErr != nil {
		t.Fatal(stageSessionRollbackErr)
	}
	runtime := &recordingRuntime{Runtime: underlying, reject: agent.ErrCommandStoreMismatch}
	err = Recover(t.Context(), runtime, store, window, retry.Backoff{})
	if !errors.Is(err, agent.ErrCommandStoreMismatch) {
		t.Fatalf("store mismatch recovery error = %v", err)
	}
	stored, exists := store.PendingSessionRollback(pending.SessionID)
	if !exists || stored.CommandID != pending.CommandID || stored.Phase != workbench.SessionRollbackPrepared {
		t.Fatalf("preserved rollback = %+v, present %t", stored, exists)
	}
}
