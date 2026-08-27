package sqlite_test

import (
	"errors"
	"testing"
	"time"

	storage "github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
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
	if reserveErr := store.Reserve(t.Context(), record); reserveErr != nil {
		t.Fatalf("Reserve: %v", reserveErr)
	}
	if reserveErr := store.Reserve(t.Context(), record); reserveErr != nil {
		t.Fatalf("Reserve exact replay: %v", reserveErr)
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

func TestChildRunStartReservationCleanupIsOwnerScopedAndBootWide(t *testing.T) {
	db, err := storage.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := storage.NewChildRunStartReservationStore(db)
	for _, record := range []storage.ChildRunStartReservationRecord{
		{MemberID: "member-a", SessionID: "session-a", Payload: []byte(`{"run":"a"}`), CreatedAt: time.Unix(1, 0)},
		{MemberID: "member-b", SessionID: "session-b", Payload: []byte(`{"run":"b"}`), CreatedAt: time.Unix(2, 0)},
	} {
		if err := store.Reserve(t.Context(), record); err != nil {
			t.Fatalf("Reserve %q: %v", record.MemberID, err)
		}
	}
	if err := store.DeleteSession(t.Context(), "session-a"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	var sessionA, sessionB int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM child_run_start_reservations WHERE session_id = ?`, "session-a",
	).Scan(&sessionA); err != nil {
		t.Fatalf("count session-a: %v", err)
	}
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM child_run_start_reservations WHERE session_id = ?`, "session-b",
	).Scan(&sessionB); err != nil {
		t.Fatalf("count session-b: %v", err)
	}
	if sessionA != 0 || sessionB != 1 {
		t.Fatalf("owner cleanup = session-a:%d session-b:%d, want 0/1", sessionA, sessionB)
	}
	if err := store.DeleteAll(t.Context()); err != nil {
		t.Fatalf("DeleteAll: %v", err)
	}
	var remaining int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM child_run_start_reservations`,
	).Scan(&remaining); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("reservations after DeleteAll = %d, want 0", remaining)
	}
}
