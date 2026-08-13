package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/idempotency"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

func TestIdempotencyStoreReplayConflictAndExpiry(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewIdempotencyStore(db)
	ctx := context.Background()

	first := idempotency.Record{Key: "request-key", Fingerprint: "first", Payload: []byte(`{"result":1}`)}
	got, claimed, err := store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || !claimed {
		t.Fatalf("claim first record: claimed=%v err=%v", claimed, err)
	}
	if len(got.Payload) != 0 {
		t.Fatalf("new claim payload = %q, want empty", got.Payload)
	}
	if _, err := db.ExecContext(ctx, `UPDATE idempotency_records SET expires_at = 0 WHERE key = ?`, first.Key); err != nil {
		t.Fatalf("age pending claim: %v", err)
	}
	got, claimed, err = store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || claimed || len(got.Payload) != 0 {
		t.Fatalf("aged pending claim: record=%+v claimed=%v err=%v", got, claimed, err)
	}
	conflicting := idempotency.Record{Key: first.Key, Fingerprint: "second"}
	if _, _, err := store.Claim(ctx, conflicting.Key, conflicting.Fingerprint); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("reuse aged pending claim = %v, want ErrKeyConflict", err)
	}
	if err := store.Complete(ctx, first); err != nil {
		t.Fatalf("complete aged pending record: %v", err)
	}
	got, claimed, err = store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || claimed || string(got.Payload) != string(first.Payload) {
		t.Fatalf("completed claim: record=%+v claimed=%v err=%v", got, claimed, err)
	}
	if _, _, err := store.Claim(ctx, conflicting.Key, conflicting.Fingerprint); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("claim conflicting record = %v, want ErrKeyConflict", err)
	}

	if _, err := db.ExecContext(ctx, `UPDATE idempotency_records SET expires_at = 0 WHERE key = ?`, first.Key); err != nil {
		t.Fatalf("expire record: %v", err)
	}
	got, claimed, err = store.Claim(ctx, conflicting.Key, conflicting.Fingerprint)
	if err != nil || !claimed || got.Fingerprint != conflicting.Fingerprint {
		t.Fatalf("replace expired record: record=%+v claimed=%v err=%v", got, claimed, err)
	}
}

func TestIdempotencyStoreKeepsAbandonedClaimAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	store := sqlite.NewIdempotencyStore(db)
	record, claimed, err := store.Claim(t.Context(), "abandoned", "first")
	if err != nil || !claimed {
		t.Fatalf("initial claim = (%+v, %v, %v)", record, claimed, err)
	}
	if _, err := db.ExecContext(t.Context(),
		`UPDATE idempotency_records SET expires_at = 0 WHERE key = ?`, record.Key,
	); err != nil {
		t.Fatalf("age pending claim: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close before reopen: %v", err)
	}

	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store = sqlite.NewIdempotencyStore(db)
	got, claimed, err := store.Claim(t.Context(), record.Key, record.Fingerprint)
	if err != nil || claimed || len(got.Payload) != 0 {
		t.Fatalf("claim after reopen = (%+v, %v, %v), want unresolved reservation", got, claimed, err)
	}
	if _, _, err := store.Claim(t.Context(), record.Key, "second"); !errors.Is(err, idempotency.ErrKeyConflict) {
		t.Fatalf("reuse after reopen = %v, want ErrKeyConflict", err)
	}
}

func TestIdempotencyNamespaceIdentifiesOneDurableStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lyra.db")
	openNamespace := func() (*sql.DB, string) {
		db, err := sqlite.Open(t.Context(), path)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		namespace, err := sqlite.IdempotencyNamespace(t.Context(), db)
		if err != nil {
			_ = db.Close()
			t.Fatalf("read namespace: %v", err)
		}
		return db, namespace
	}

	db, first := openNamespace()
	if err := db.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}
	db, reopened := openNamespace()
	if reopened != first {
		t.Fatalf("reopened namespace = %q, want %q", reopened, first)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close reopened: %v", err)
	}

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("remove recreated-store fixture %q: %v", path+suffix, err)
		}
	}
	db, replaced := openNamespace()
	t.Cleanup(func() { _ = db.Close() })
	if replaced == first {
		t.Fatalf("replacement store reused namespace %q", replaced)
	}
}

func TestIdempotencyNamespaceRejectsCorruptIdentity(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.ExecContext(t.Context(),
		`UPDATE runtime_identity SET idempotency_namespace = 'not-an-opaque-identity' WHERE id = 1`,
	); err != nil {
		t.Fatalf("corrupt namespace fixture: %v", err)
	}
	if _, err := sqlite.IdempotencyNamespace(t.Context(), db); err == nil {
		t.Fatal("read corrupt namespace succeeded")
	}
}
