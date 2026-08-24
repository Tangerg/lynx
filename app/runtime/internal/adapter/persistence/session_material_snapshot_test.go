package persistence

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/goal"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/run"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

type blockingGoalProjection struct {
	*sqlite.GoalStore
	entered chan struct{}
	release chan struct{}
}

func (projection *blockingGoalProjection) Get(ctx context.Context, sessionID string) (goal.Goal, bool, error) {
	close(projection.entered)
	select {
	case <-projection.release:
	case <-ctx.Done():
		return goal.Goal{}, false, ctx.Err()
	}
	return projection.GoalStore.Get(ctx, sessionID)
}

func TestReadMaterialSnapshotKeepsSessionPlanAndGoalOnOneTransaction(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "runtime.db")
	readerDB, err := sqlite.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatalf("open reader database: %v", err)
	}
	t.Cleanup(func() { _ = readerDB.Close() })
	writerDB, err := sqlite.Open(t.Context(), databasePath)
	if err != nil {
		t.Fatalf("open writer database: %v", err)
	}
	t.Cleanup(func() { _ = writerDB.Close() })
	ctx := t.Context()
	createdAt := time.Date(2026, 8, 14, 1, 0, 0, 0, time.UTC)
	readerSessionStore := sqlite.NewSessionStore(readerDB)
	writerSessionStore := sqlite.NewSessionStore(writerDB)
	original := sessionfixture.MustRestore(session.Snapshot{
		ID: "ses_snapshot", Workspace: sessionfixture.MustWorkspace("/workspace"), Title: "before",
		StartedAt: createdAt, UpdatedAt: createdAt, Revision: 1,
	})
	if err := writerSessionStore.Insert(ctx, original); err != nil {
		t.Fatalf("seed Session: %v", err)
	}
	readerPlanStore := sqlite.NewPlanStore(readerDB)
	writerPlanStore := sqlite.NewPlanStore(writerDB)
	originalPlan, err := (plan.State{}).Replace([]plan.Step{{
		Description: "before", Status: plan.StatusInProgress,
	}}, createdAt)
	if err != nil {
		t.Fatalf("prepare Plan: %v", err)
	}
	if err := writerPlanStore.Save(ctx, original.ID(), 0, originalPlan); err != nil {
		t.Fatalf("seed Plan: %v", err)
	}
	readerGoalStore := sqlite.NewGoalStore(readerDB)
	writerGoalStore := sqlite.NewGoalStore(writerDB)
	originalGoal, err := goal.New(
		original.ID(), "before", modelref.Selection{}, goal.Budget{}, run.Capabilities{},
		"goal_before", createdAt,
	)
	if err != nil {
		t.Fatalf("prepare Goal: %v", err)
	}
	originalGoal, applied, err := writerGoalStore.Save(ctx, originalGoal, goal.Version{})
	if err != nil || !applied {
		t.Fatalf("seed Goal: applied=%t err=%v", applied, err)
	}
	blockingGoal := &blockingGoalProjection{
		GoalStore: readerGoalStore, entered: make(chan struct{}), release: make(chan struct{}),
	}
	stores := NewSessionStores(SessionStoresConfig{
		Sessions: readerSessionStore, Transcript: sqlite.NewTranscriptStore(readerDB),
		Interrupts: NewInterruptStore(sqlite.NewInterruptStore(readerDB)),
		Runs:       sqlite.NewRunStore(readerDB), Plan: readerPlanStore, Goals: blockingGoal,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, readerDB, fn)
		},
	})

	snapshotResult := make(chan struct {
		snapshotRevision uint64
		planRevision     uint64
		goalRevision     int64
		err              error
	}, 1)
	go func() {
		snapshot, err := stores.ReadMaterialSnapshot(ctx, original.ID())
		snapshotResult <- struct {
			snapshotRevision uint64
			planRevision     uint64
			goalRevision     int64
			err              error
		}{snapshot.Session.Revision(), snapshot.Plan.Revision(), snapshot.Goal.Revision, err}
	}()
	<-blockingGoal.entered

	updatedTitle := "after"
	updatedSession, changed, err := original.Apply(session.Patch{
		Title: &updatedTitle, ExpectedRevision: original.Revision(),
	}, createdAt.Add(time.Second))
	if err != nil || !changed {
		t.Fatalf("prepare Session replacement: changed=%t err=%v", changed, err)
	}
	updatedPlan, err := originalPlan.Replace([]plan.Step{{
		Description: "after", Status: plan.StatusCompleted,
	}}, createdAt.Add(time.Second))
	if err != nil {
		t.Fatalf("prepare Plan replacement: %v", err)
	}
	updatedGoal := originalGoal.Clone()
	updatedGoal.Pause(goal.ReasonRuntimeRestarted, "", createdAt.Add(time.Second))
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- sqlite.RunInTx(ctx, writerDB, func(ctx context.Context) error {
			if err := writerSessionStore.Save(ctx, original.Revision(), updatedSession); err != nil {
				return err
			}
			if err := writerPlanStore.Save(ctx, original.ID(), originalPlan.Revision(), updatedPlan); err != nil {
				return err
			}
			_, applied, err := writerGoalStore.Save(ctx, updatedGoal, originalGoal.Version())
			if err == nil && !applied {
				return errors.New("replace Goal: CAS did not apply")
			}
			return err
		})
	}()
	if err := <-writerDone; err != nil {
		t.Fatalf("commit concurrent successor state: %v", err)
	}
	close(blockingGoal.release)

	read := <-snapshotResult
	if read.err != nil {
		t.Fatalf("ReadMaterialSnapshot: %v", read.err)
	}
	if read.snapshotRevision != 1 || read.planRevision != 1 || read.goalRevision != 1 {
		t.Fatalf(
			"snapshot revisions = Session:%d Plan:%d Goal:%d, want 1/1/1",
			read.snapshotRevision, read.planRevision, read.goalRevision,
		)
	}
	stores.goals = readerGoalStore
	after, err := stores.ReadMaterialSnapshot(ctx, original.ID())
	if err != nil {
		t.Fatalf("read successor snapshot: %v", err)
	}
	if after.Session.Revision() != 2 || after.Plan.Revision() != 2 || after.Goal.Revision != 2 {
		t.Fatalf(
			"successor revisions = Session:%d Plan:%d Goal:%d, want 2/2/2",
			after.Session.Revision(), after.Plan.Revision(), after.Goal.Revision,
		)
	}
}
