package sqlite_test

import (
	"errors"
	"testing"
	"time"

	storage "github.com/Tangerg/lynx/app/runtime/internal/infra/sqlite"
)

func TestChildRunStartReservationRetainsIdempotentConclusion(t *testing.T) {
	db, err := storage.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := storage.NewChildRunStartReservationStore(db)
	record := storage.ChildRunStartReservationRecord{
		MemberID: "member-child", SessionID: "session-1",
		Payload: []byte(`{"run":"child"}`), CreatedAt: time.Unix(1, 2).UTC(),
	}
	if err := store.Reserve(t.Context(), record); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if err := store.Reserve(t.Context(), record); err != nil {
		t.Fatalf("Reserve exact replay: %v", err)
	}
	changed, err := store.Conclude(
		t.Context(), record, storage.ChildRunStartConclusionStarted,
	)
	if err != nil || !changed {
		t.Fatalf("first started conclusion = (%v, %v), want changed", changed, err)
	}
	changed, err = store.Conclude(
		t.Context(), record, storage.ChildRunStartConclusionStarted,
	)
	if err != nil || changed {
		t.Fatalf("started replay = (%v, %v), want unchanged success", changed, err)
	}
	if _, err := store.Conclude(
		t.Context(), record, storage.ChildRunStartConclusionAborted,
	); !errors.Is(err, storage.ErrChildRunStartReservationConflict) {
		t.Fatalf("contradictory conclusion = %v, want conflict", err)
	}
	var state string
	if err := db.QueryRowContext(
		t.Context(),
		`SELECT state FROM child_run_start_reservations WHERE member_id = ?`,
		record.MemberID,
	).Scan(&state); err != nil || state != "started" {
		t.Fatalf("stored conclusion = %q, %v; want started", state, err)
	}
}

func TestChildRunStartReservationRejectsMissingAndChangedIdentity(t *testing.T) {
	db, err := storage.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := storage.NewChildRunStartReservationStore(db)
	record := storage.ChildRunStartReservationRecord{
		MemberID: "member-child", SessionID: "session-1",
		Payload: []byte(`{"run":"child"}`), CreatedAt: time.Unix(1, 2).UTC(),
	}
	if _, err := store.Conclude(
		t.Context(), record, storage.ChildRunStartConclusionAborted,
	); !errors.Is(err, storage.ErrChildRunStartReservationConflict) {
		t.Fatalf("missing conclusion = %v, want conflict", err)
	}
	if err := store.Reserve(t.Context(), record); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	changed := record
	changed.Payload = []byte(`{"run":"other"}`)
	if err := store.Reserve(t.Context(), changed); !errors.Is(err, storage.ErrChildRunStartReservationConflict) {
		t.Fatalf("changed replay = %v, want conflict", err)
	}
}
