package sqlite_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Tangerg/lynx/app/runtime/internal/component/idempotency"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func TestIdempotencyStoreReplayConflictAndExpiry(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
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
	got, claimed, err = store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || claimed || len(got.Payload) != 0 {
		t.Fatalf("pending claim: record=%+v claimed=%v err=%v", got, claimed, err)
	}
	if err := store.Complete(ctx, first); err != nil {
		t.Fatalf("complete first record: %v", err)
	}
	got, claimed, err = store.Claim(ctx, first.Key, first.Fingerprint)
	if err != nil || claimed || string(got.Payload) != string(first.Payload) {
		t.Fatalf("completed claim: record=%+v claimed=%v err=%v", got, claimed, err)
	}
	conflicting := idempotency.Record{Key: first.Key, Fingerprint: "second"}
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
