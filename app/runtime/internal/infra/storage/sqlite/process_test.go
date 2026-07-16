package sqlite_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/lynx/agent"
	"github.com/Tangerg/lynx/agent/core"
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

func TestProcessStoreSaveLoadCAS(t *testing.T) {
	ctx := context.Background()
	store := newProcessStore(t)
	snapshot := validStoredSnapshot("proc-1", core.StatusWaiting)
	snapshot.Conditions = map[string]bool{"k": true}

	revision, err := store.Save(ctx, snapshot, 0)
	if err != nil || revision != 1 {
		t.Fatalf("first Save = revision %d, err %v", revision, err)
	}
	stale := snapshot
	if _, err := store.Save(ctx, stale, 0); !errors.Is(err, core.ErrRevisionConflict) {
		t.Fatalf("stale Save error = %v", err)
	}
	snapshot.Revision = revision
	snapshot.Status = core.StatusCompleted
	snapshot.Suspension = nil
	revision, err = store.Save(ctx, snapshot, revision)
	if err != nil || revision != 2 {
		t.Fatalf("second Save = revision %d, err %v", revision, err)
	}

	got, err := store.Load(ctx, snapshot.ID)
	if err != nil || got.Revision != 2 || got.Status != core.StatusCompleted || !got.Conditions["k"] {
		t.Fatalf("Load = %+v, err %v", got, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for tokens := 10; tokens <= 11; tokens++ {
		candidate := got
		candidate.Tokens = tokens
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, saveErr := store.Save(ctx, candidate, 2)
			results <- saveErr
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		if result == nil {
			successes++
		} else if errors.Is(result, core.ErrRevisionConflict) {
			conflicts++
		} else {
			t.Fatalf("concurrent Save error = %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent CAS successes=%d conflicts=%d", successes, conflicts)
	}
}

func TestProcessStoreLoadMissingIsSentinel(t *testing.T) {
	store := newProcessStore(t)
	if _, err := store.Load(context.Background(), "nope"); !errors.Is(err, core.ErrSnapshotNotFound) {
		t.Fatalf("Load(missing) err = %v", err)
	}
}

func TestProcessStoreListDeleteIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newProcessStore(t)
	for _, id := range []string{"a", "b"} {
		if _, err := store.Save(ctx, validStoredSnapshot(id, core.StatusCompleted), 0); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := store.List(ctx)
	if err != nil || len(ids) != 2 {
		t.Fatalf("List = %v, err %v", ids, err)
	}
	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, "a"); err != nil {
		t.Fatalf("idempotent Delete: %v", err)
	}
	if ids, _ = store.List(ctx); len(ids) != 1 || ids[0] != "b" {
		t.Fatalf("after delete List = %v", ids)
	}
}
