package sqlite_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/idempotency"
	lyrasqlite "github.com/Tangerg/lynx/app2/runtime/sqlite"
)

func TestIdempotencyStorePersistsReplayConflictAndPendingAcrossReopen(t *testing.T) {
	root := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	path := filepath.Join(root, "runtime.sqlite")
	database, err := lyrasqlite.Open(t.Context(), lyrasqlite.Config{
		Path: path, CreatedByVersion: "test",
	})
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	store, err := lyrasqlite.NewIdempotencyStore(database, 24*time.Hour)
	if err != nil {
		t.Fatalf("NewIdempotencyStore() error = %v", err)
	}

	completed := idempotency.Record{
		Key: "complete", Fingerprint: "request-a", Payload: []byte(`{"version":1}`),
	}
	if _, claimed, err := store.Claim(t.Context(), completed.Key, completed.Fingerprint); err != nil || !claimed {
		t.Fatalf("initial complete claim = claimed:%v error:%v", claimed, err)
	}
	if _, err := store.Complete(t.Context(), completed); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if _, claimed, err := store.Claim(t.Context(), "pending", "request-b"); err != nil || !claimed {
		t.Fatalf("pending claim = claimed:%v error:%v", claimed, err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	database, err = lyrasqlite.Open(t.Context(), lyrasqlite.Config{
		Path: path, CreatedByVersion: "reopened",
	})
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	store, err = lyrasqlite.NewIdempotencyStore(database, 24*time.Hour)
	if err != nil {
		t.Fatalf("reopened NewIdempotencyStore() error = %v", err)
	}

	replayed, claimed, err := store.Claim(t.Context(), completed.Key, completed.Fingerprint)
	if err != nil || claimed || string(replayed.Payload) != string(completed.Payload) {
		t.Fatalf("replayed record = %+v, claimed:%v error:%v", replayed, claimed, err)
	}
	if _, _, err := store.Claim(t.Context(), completed.Key, "different"); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("conflicting claim error = %v", err)
	}
	pending, claimed, err := store.Claim(t.Context(), "pending", "request-b")
	if err != nil || claimed || len(pending.Payload) != 0 {
		t.Fatalf("pending replay = %+v, claimed:%v error:%v", pending, claimed, err)
	}
}
