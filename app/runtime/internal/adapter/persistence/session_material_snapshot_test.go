package persistence

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/plan"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
	"github.com/Tangerg/lynx/app/runtime/internal/testsupport/sessionfixture"
)

type blockingPlanProjection struct {
	*sqlite.PlanStore
	entered chan struct{}
	release chan struct{}
}

func (projection *blockingPlanProjection) State(ctx context.Context, sessionID string) (plan.State, error) {
	close(projection.entered)
	select {
	case <-projection.release:
	case <-ctx.Done():
		return plan.State{}, ctx.Err()
	}
	return projection.PlanStore.State(ctx, sessionID)
}

func TestReadMaterialSnapshotKeepsSessionAndPlanOnOneTransaction(t *testing.T) {
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
		ID: "ses_snapshot", CWD: "/workspace", Title: "before",
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
	blockingPlan := &blockingPlanProjection{
		PlanStore: readerPlanStore, entered: make(chan struct{}), release: make(chan struct{}),
	}
	stores := NewSessionStores(SessionStoresConfig{
		Sessions: readerSessionStore, Transcript: sqlite.NewTranscriptStore(readerDB),
		Interrupts: NewInterruptStore(sqlite.NewInterruptStore(readerDB)),
		Runs:       sqlite.NewRunStore(readerDB), Plan: blockingPlan,
		Tx: func(ctx context.Context, fn func(context.Context) error) error {
			return sqlite.RunInTx(ctx, readerDB, fn)
		},
	})

	snapshotResult := make(chan struct {
		snapshotRevision uint64
		planRevision     uint64
		err              error
	}, 1)
	go func() {
		snapshot, err := stores.ReadMaterialSnapshot(ctx, original.ID())
		snapshotResult <- struct {
			snapshotRevision uint64
			planRevision     uint64
			err              error
		}{snapshot.Session.Revision(), snapshot.Plan.Revision(), err}
	}()
	<-blockingPlan.entered

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
	writerDone := make(chan error, 1)
	go func() {
		writerDone <- sqlite.RunInTx(ctx, writerDB, func(ctx context.Context) error {
			if err := writerSessionStore.Save(ctx, original.Revision(), updatedSession); err != nil {
				return err
			}
			return writerPlanStore.Save(ctx, original.ID(), originalPlan.Revision(), updatedPlan)
		})
	}()
	if err := <-writerDone; err != nil {
		t.Fatalf("commit concurrent successor state: %v", err)
	}
	close(blockingPlan.release)

	read := <-snapshotResult
	if read.err != nil {
		t.Fatalf("ReadMaterialSnapshot: %v", read.err)
	}
	if read.snapshotRevision != 1 || read.planRevision != 1 {
		t.Fatalf("snapshot revisions = Session:%d Plan:%d, want 1/1", read.snapshotRevision, read.planRevision)
	}
	stores.plan = readerPlanStore
	after, err := stores.ReadMaterialSnapshot(ctx, original.ID())
	if err != nil {
		t.Fatalf("read successor snapshot: %v", err)
	}
	if after.Session.Revision() != 2 || after.Plan.Revision() != 2 {
		t.Fatalf("successor revisions = Session:%d Plan:%d, want 2/2", after.Session.Revision(), after.Plan.Revision())
	}
}
