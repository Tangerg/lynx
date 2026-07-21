package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/storetest"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newProcessStore(t *testing.T) *sqlite.ProcessStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewProcessStore(db)
}

func validStoredSnapshot(id string, status core.ProcessStatus) core.ProcessSnapshot {
	started := time.Date(2026, time.July, 16, 8, 0, 0, 0, time.UTC)
	snapshot := core.ProcessSnapshot{
		SchemaVersion: core.ProcessSnapshotSchemaVersion,
		ID:            id,
		Deployment:    core.DeploymentRef{Name: "chat", Digest: "digest"},
		StartedAt:     started,
		CapturedAt:    started.Add(time.Second),
		Status:        status,
	}
	if status == core.StatusWaiting {
		snapshot.Suspension = &agent.Suspension{
			SchemaVersion: agent.SuspensionSchemaVersion,
			ID:            "suspension-" + id,
			Kind:          agent.SuspensionTool,
			Prompt:        json.RawMessage(`"continue?"`),
			ResumeSchema:  json.RawMessage(`{"type":"boolean"}`),
			Payload:       json.RawMessage(`{"checkpoint":true}`),
			CreatedAt:     started,
		}
	}
	return snapshot
}

func storedSnapshotChange(rootID string, snapshots ...core.ProcessSnapshot) core.ProcessSnapshotChange {
	return core.ProcessSnapshotChange{Tree: &core.ProcessSnapshotTree{
		RootID:    rootID,
		Snapshots: snapshots,
	}}
}

func TestProcessStoreApplyLoadReplacement(t *testing.T) {
	ctx := context.Background()
	store := newProcessStore(t)
	snapshot := validStoredSnapshot("proc-1", core.StatusWaiting)
	snapshot.Conditions = map[string]bool{"k": true}

	if err := store.Apply(ctx, storedSnapshotChange(snapshot.ID, snapshot)); err != nil {
		t.Fatalf("first Apply: %v", err)
	}
	snapshot.Status = core.StatusCompleted
	snapshot.Suspension = nil
	if err := store.Apply(ctx, storedSnapshotChange(snapshot.ID, snapshot)); err != nil {
		t.Fatalf("second Apply: %v", err)
	}

	got, err := store.Load(ctx, snapshot.ID)
	if err != nil || got.Status != core.StatusCompleted || !got.Conditions["k"] {
		t.Fatalf("Load = %+v, err %v", got, err)
	}
}

func TestProcessStoreContract(t *testing.T) {
	if err := storetest.TestProcessStore(t.Context(), newProcessStore(t)); err != nil {
		t.Fatal(err)
	}
}

func TestProcessStoreLoadMissingIsSentinel(t *testing.T) {
	store := newProcessStore(t)
	if _, err := store.Load(context.Background(), "nope"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load(missing) err = %v", err)
	}
}

func TestProcessStoreLoadCorruptSnapshotIsInvalidSentinel(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewProcessStore(db)
	snapshot := validStoredSnapshot("proc-corrupt", core.StatusCompleted)
	if err := store.Apply(t.Context(), storedSnapshotChange(snapshot.ID, snapshot)); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE process_snapshots SET snapshot = ? WHERE id = ?`, `{`, snapshot.ID); err != nil {
		t.Fatalf("corrupt stored snapshot: %v", err)
	}
	if _, err := store.Load(t.Context(), snapshot.ID); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("Load(corrupt) err = %v, want ErrInvalidSnapshot", err)
	}
}

func TestProcessStoreDeleteIgnoresUnrelatedCorruptSnapshot(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewProcessStore(db)
	corrupt := validStoredSnapshot("corrupt", core.StatusCompleted)
	target := validStoredSnapshot("target", core.StatusCompleted)
	if err := store.Apply(t.Context(), storedSnapshotChange(corrupt.ID, corrupt)); err != nil {
		t.Fatalf("Apply corrupt target: %v", err)
	}
	if err := store.Apply(t.Context(), storedSnapshotChange(target.ID, target)); err != nil {
		t.Fatalf("Apply delete target: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE process_snapshots SET snapshot = ? WHERE id = ?`, `{`, corrupt.ID); err != nil {
		t.Fatalf("corrupt stored snapshot: %v", err)
	}

	if err := store.Apply(t.Context(), core.ProcessSnapshotChange{DeleteRoots: []string{target.ID}}); err != nil {
		t.Fatalf("Delete target: %v", err)
	}
	if _, err := store.Load(t.Context(), target.ID); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load deleted target: %v", err)
	}
	if _, err := store.Load(t.Context(), corrupt.ID); !errors.Is(err, core.ErrInvalidSnapshot) {
		t.Fatalf("unrelated corrupt snapshot changed: %v", err)
	}
}

func TestProcessStoreListDelete(t *testing.T) {
	ctx := context.Background()
	store := newProcessStore(t)
	for _, id := range []string{"a", "b"} {
		if err := store.Apply(ctx, storedSnapshotChange(id, validStoredSnapshot(id, core.StatusCompleted))); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.List(ctx)
	if err != nil || len(ids) != 2 {
		t.Fatalf("List = %v, err %v", ids, err)
	}
	if err := store.Apply(ctx, core.ProcessSnapshotChange{DeleteRoots: []string{"a"}}); err != nil {
		t.Fatal(err)
	}
	if ids, _ = store.List(ctx); len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("after delete List = %v", ids)
	}
}

func TestProcessStoreDeleteTreeRemovesDescendantsOnly(t *testing.T) {
	ctx := t.Context()
	store := newProcessStore(t)
	root := validStoredSnapshot("root", core.StatusCompleted)
	child := validStoredSnapshot("child", core.StatusKilled)
	child.ParentID = root.ID
	child.Depth = 1
	grandchild := validStoredSnapshot("grandchild", core.StatusKilled)
	grandchild.ParentID = child.ID
	grandchild.Depth = 2
	unrelated := validStoredSnapshot("unrelated", core.StatusCompleted)
	if err := store.Apply(ctx, storedSnapshotChange(root.ID, root, child, grandchild)); err != nil {
		t.Fatalf("Apply tree: %v", err)
	}
	if err := store.Apply(ctx, storedSnapshotChange(unrelated.ID, unrelated)); err != nil {
		t.Fatalf("Apply unrelated: %v", err)
	}

	if err := store.Apply(ctx, core.ProcessSnapshotChange{DeleteRoots: []string{root.ID}}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	ids, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(ids) != 1 || ids[0] != unrelated.ID {
		t.Fatalf("remaining snapshots = %v, want only unrelated", ids)
	}
}

// A single change may re-capture a subtree while deleting an ancestor whose
// recursive descendants still include it. The store must persist the capture:
// deletes run before writes, so the cascade cannot remove a just-written node.
func TestProcessStoreApplyKeepsWrittenTreeWhenDeletingAncestor(t *testing.T) {
	ctx := t.Context()
	store := newProcessStore(t)
	ancestor := validStoredSnapshot("ancestor", core.StatusCompleted)
	target := validStoredSnapshot("target", core.StatusCompleted)
	target.ParentID = ancestor.ID
	target.Depth = 1
	if err := store.Apply(ctx, storedSnapshotChange(ancestor.ID, ancestor, target)); err != nil {
		t.Fatalf("Apply seed tree: %v", err)
	}

	recaptured := validStoredSnapshot("target", core.StatusCompleted)
	recaptured.ParentID = ancestor.ID
	recaptured.Depth = 1
	change := core.ProcessSnapshotChange{
		Tree:        &core.ProcessSnapshotTree{RootID: recaptured.ID, Snapshots: []core.ProcessSnapshot{recaptured}},
		DeleteRoots: []string{ancestor.ID},
	}
	if err := store.Apply(ctx, change); err != nil {
		t.Fatalf("Apply recapture with ancestor delete: %v", err)
	}

	if _, err := store.Load(ctx, target.ID); err != nil {
		t.Fatalf("recaptured target lost to ancestor cascade: %v", err)
	}
	if _, err := store.Load(ctx, ancestor.ID); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load ancestor after delete = %v, want ErrSnapshotNotFound", err)
	}
}
