package sqlite_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/scope/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/scope/app/runtime/internal/infra/sqlite"
)

func newScheduleStore(t *testing.T) *sqlite.ScheduleStore {
	t.Helper()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewScheduleStore(db)
}

// TestScheduleCRUD covers create (id assigned, persisted verbatim), get, the
// next-due query, update, and delete.
func TestScheduleCRUD(t *testing.T) {
	ctx := context.Background()
	s := newScheduleStore(t)

	past := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	created, err := s.Create(ctx, schedule.Schedule{
		Title: "standup", Instructions: "summarize the diff", CWD: "/proj",
		Cron: "0 9 * * 1-5", Enabled: true, NextRunAt: past,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("create did not assign an id")
	}
	if created.CreatedAt.IsZero() {
		t.Error("create did not stamp CreatedAt")
	}

	got, err := s.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Instructions != "summarize the diff" || got.Cron != "0 9 * * 1-5" || !got.Enabled {
		t.Errorf("get round-trip mismatch: %+v", got)
	}
	if !got.NextRunAt.Equal(past) {
		t.Errorf("NextRunAt = %v, want %v", got.NextRunAt, past)
	}
	if !got.LastRunAt.IsZero() {
		t.Errorf("LastRunAt = %v, want zero (never fired)", got.LastRunAt)
	}

	// Due: the past nextRunAt is in (0, now], so it's returned.
	due, err := s.Due(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due) != 1 || due[0].ID != created.ID {
		t.Fatalf("due = %+v, want the one past-due schedule", due)
	}

	// Update: disabling clears the due index (NextRunAt zero) → never due.
	got.Enabled = false
	got.NextRunAt = time.Time{}
	got.Title = "renamed"
	if _, updateErr := s.Update(ctx, got, got.Revision); updateErr != nil {
		t.Fatalf("update: %v", updateErr)
	}
	reread, _ := s.Get(ctx, created.ID)
	if reread.Enabled || reread.Title != "renamed" || !reread.NextRunAt.IsZero() {
		t.Errorf("update not applied: %+v", reread)
	}

	deleted, err := s.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !deleted {
		t.Fatal("delete reported no committed mutation")
	}
	if _, getErr := s.Get(ctx, created.ID); getErr != schedule.ErrNotFound {
		t.Errorf("get after delete err = %v, want ErrNotFound", getErr)
	}
	deleted, err = s.Delete(ctx, created.ID)
	if err != nil || deleted {
		t.Fatalf("second delete = (%v, %v), want false, nil", deleted, err)
	}
}

// TestScheduleRecordRunLeavesCursor: a manual run-now (RecordRun) updates
// LastRunAt but must NOT touch NextRunAt. Re-stamping a cursor value read before
// the worker advanced it would rewind the schedule and re-fire it every tick.
func TestScheduleRecordRunLeavesCursor(t *testing.T) {
	ctx := t.Context()
	s := newScheduleStore(t)

	past := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	created, err := s.Create(ctx, schedule.Schedule{
		Instructions: "p", Cron: "@daily", Enabled: true, NextRunAt: past,
	})
	if err != nil {
		t.Fatal(err)
	}

	future := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond)
	firedAt := time.Now().UTC().Truncate(time.Millisecond)
	occurrence := schedule.Occurrence{
		ID: created.ID + ":scheduled", Schedule: created,
		DueAt: created.NextRunAt, FiredAt: firedAt, NextRunAt: future,
		SessionID: "ses_scheduled", RunID: "run_scheduled",
	}
	claimed, err := s.Claim(ctx, occurrence)
	if err != nil || !claimed {
		t.Fatalf("Claim = (%v, %v), want true, nil", claimed, err)
	}
	if err := s.Accept(ctx, occurrence.ID, occurrence.RunID); err != nil {
		t.Fatalf("Accept: %v", err)
	}

	ranAt := time.Now().UTC().Truncate(time.Millisecond)
	if err := s.RecordRun(ctx, created.ID, ranAt); err != nil {
		t.Fatalf("recordRun: %v", err)
	}

	got, _ := s.Get(ctx, created.ID)
	if !got.NextRunAt.Equal(future) {
		t.Errorf("RecordRun rewound NextRunAt to %v, want %v (cursor untouched)", got.NextRunAt, future)
	}
	if !got.LastRunAt.Equal(ranAt) {
		t.Errorf("LastRunAt = %v, want %v", got.LastRunAt, ranAt)
	}
	due, _ := s.Due(ctx, time.Now(), 100)
	if len(due) != 0 {
		t.Errorf("due after RecordRun = %+v, want none (cursor still in the future)", due)
	}
}

func TestScheduleClaimRejectsStaleRevisionWithUnchangedCursor(t *testing.T) {
	ctx := t.Context()
	store := newScheduleStore(t)
	dueAt := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	nextAt := dueAt.Add(time.Hour)
	created, err := store.Create(ctx, schedule.Schedule{
		Instructions: "old instructions", Cron: "0 * * * *", Enabled: true, NextRunAt: dueAt,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	due, err := store.Due(ctx, dueAt, 1)
	if err != nil || len(due) != 1 {
		t.Fatalf("Due = (%+v, %v), want one schedule", due, err)
	}

	updated := created
	updated.Instructions = "new instructions"
	updated, err = store.Update(ctx, updated, created.Revision)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if !updated.NextRunAt.Equal(dueAt) {
		t.Fatalf("Update changed cursor to %v, want %v", updated.NextRunAt, dueAt)
	}

	stale := schedule.Occurrence{
		ID: created.ID + ":stale", Schedule: due[0],
		DueAt: dueAt, FiredAt: dueAt, NextRunAt: nextAt,
		SessionID: "ses_stale", RunID: "run_stale",
	}
	claimed, err := store.Claim(ctx, stale)
	if err != nil || claimed {
		t.Fatalf("stale Claim = (%v, %v), want false, nil", claimed, err)
	}

	fresh := stale
	fresh.ID = created.ID + ":fresh"
	fresh.Schedule = updated
	fresh.SessionID = "ses_fresh"
	fresh.RunID = "run_fresh"
	claimed, err = store.Claim(ctx, fresh)
	if err != nil || !claimed {
		t.Fatalf("fresh Claim = (%v, %v), want true, nil", claimed, err)
	}
	pending, err := store.Pending(ctx, 1)
	if err != nil || len(pending) != 1 || pending[0].Schedule.Instructions != "new instructions" {
		t.Fatalf("Pending = (%+v, %v), want the updated snapshot", pending, err)
	}
}

func TestScheduleOccurrenceSurvivesDispatchAndAcceptsOnce(t *testing.T) {
	ctx := t.Context()
	store := newScheduleStore(t)
	dueAt := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	nextAt := dueAt.Add(time.Hour)
	created, err := store.Create(ctx, schedule.Schedule{
		Title: "hourly", Instructions: "review", Cron: "0 * * * *", Enabled: true, NextRunAt: dueAt,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	occurrence := schedule.Occurrence{
		ID: created.ID + ":" + "1784960400000", Schedule: created,
		DueAt: dueAt, FiredAt: dueAt.Add(time.Minute), NextRunAt: nextAt,
		SessionID: "ses_occurrence", RunID: "run_occurrence",
	}
	claimed, err := store.Claim(ctx, occurrence)
	if err != nil || !claimed {
		t.Fatalf("Claim = (%v, %v), want true, nil", claimed, err)
	}
	claimed, err = store.Claim(ctx, occurrence)
	if err != nil || claimed {
		t.Fatalf("repeat Claim = (%v, %v), want false, nil", claimed, err)
	}
	pending, err := store.Pending(ctx, 100)
	if err != nil || len(pending) != 1 || pending[0].RunID != occurrence.RunID || pending[0].SessionID != occurrence.SessionID {
		t.Fatalf("Pending = (%+v, %v), want persisted occurrence", pending, err)
	}
	got, err := store.Get(ctx, created.ID)
	if err != nil || !got.NextRunAt.Equal(nextAt) || !got.LastRunAt.IsZero() {
		t.Fatalf("schedule after Claim = (%+v, %v), want advanced cursor and no accepted run", got, err)
	}
	if acceptErr := store.Accept(ctx, occurrence.ID, occurrence.RunID); acceptErr != nil {
		t.Fatalf("Accept: %v", acceptErr)
	}
	if pending, err = store.Pending(ctx, 100); err != nil || len(pending) != 0 {
		t.Fatalf("Pending after Accept = (%+v, %v), want empty", pending, err)
	}
	got, err = store.Get(ctx, created.ID)
	if err != nil || !got.NextRunAt.Equal(nextAt) || !got.LastRunAt.Equal(occurrence.FiredAt) {
		t.Fatalf("schedule after Claim = (%+v, %v), want advanced cursor", got, err)
	}
}

func TestScheduleClaimKeepsOnlyOnePendingOccurrencePerSchedule(t *testing.T) {
	ctx := t.Context()
	store := newScheduleStore(t)
	firstDueAt := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	secondDueAt := firstDueAt.Add(time.Hour)
	thirdDueAt := secondDueAt.Add(time.Hour)
	created, err := store.Create(ctx, schedule.Schedule{
		Title: "hourly", Instructions: "review", Cron: "0 * * * *", Enabled: true, NextRunAt: firstDueAt,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	first := schedule.Occurrence{
		ID: created.ID + ":first", Schedule: created,
		DueAt: firstDueAt, FiredAt: firstDueAt, NextRunAt: secondDueAt,
		SessionID: "ses_first", RunID: "run_first",
	}
	claimed, err := store.Claim(ctx, first)
	if err != nil || !claimed {
		t.Fatalf("first Claim = (%v, %v), want true, nil", claimed, err)
	}
	current, err := store.Get(ctx, created.ID)
	if err != nil || !current.NextRunAt.Equal(secondDueAt) {
		t.Fatalf("schedule cursor = (%v, %v), want %v", current.NextRunAt, err, secondDueAt)
	}

	// The worker can observe the later cron slot while this first occurrence is
	// still waiting for Run admission. It must not advance the cursor again or
	// materialize a second recovery item for the same schedule.
	second := schedule.Occurrence{
		ID: created.ID + ":second", Schedule: current,
		DueAt: secondDueAt, FiredAt: secondDueAt, NextRunAt: thirdDueAt,
		SessionID: "ses_second", RunID: "run_second",
	}
	claimed, err = store.Claim(ctx, second)
	if err != nil || claimed {
		t.Fatalf("second Claim = (%v, %v), want false, nil while first is pending", claimed, err)
	}
	pending, err := store.Pending(ctx, 100)
	if err != nil || len(pending) != 1 || pending[0].ID != first.ID {
		t.Fatalf("Pending = (%+v, %v), want only first occurrence", pending, err)
	}

	if acceptErr := store.Accept(ctx, first.ID, first.RunID); acceptErr != nil {
		t.Fatalf("Accept first occurrence: %v", acceptErr)
	}
	second.Schedule, err = store.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get after Accept: %v", err)
	}
	claimed, err = store.Claim(ctx, second)
	if err != nil || !claimed {
		t.Fatalf("second Claim after accept = (%v, %v), want true, nil", claimed, err)
	}
}

func TestScheduleStoreRejectsDuplicatePendingRows(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewScheduleStore(db)
	dueAt := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	created, err := store.Create(ctx, schedule.Schedule{
		Instructions: "review", Cron: "@hourly", Enabled: true, NextRunAt: dueAt,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	insert := func(id, sessionID, runID string) error {
		_, err := db.ExecContext(ctx, `INSERT INTO schedule_firings(
			id, schedule_id, instructions, cron, due_at, fired_at, next_run_at, session_id, run_id, state
		) VALUES (?, ?, 'review', '@hourly', ?, ?, ?, ?, ?, 'pending')`,
			id, created.ID, dueAt.UnixMilli(), dueAt.UnixMilli(), dueAt.Add(time.Hour).UnixMilli(), sessionID, runID)
		return err
	}
	if err := insert("first", "ses_first", "run_first"); err != nil {
		t.Fatalf("insert first pending occurrence: %v", err)
	}
	if err := insert("second", "ses_second", "run_second"); err == nil {
		t.Fatal("second pending occurrence inserted despite the per-schedule invariant")
	}
}

// TestScheduleUpdateNotFound: updating an unknown id reports ErrNotFound.
func TestScheduleUpdateNotFound(t *testing.T) {
	s := newScheduleStore(t)
	_, err := s.Update(context.Background(), schedule.Schedule{ID: "sch_nope", Instructions: "x", Cron: "@daily"}, 1)
	if err != schedule.ErrNotFound {
		t.Errorf("update unknown id err = %v, want ErrNotFound", err)
	}
}

// TestScheduleDueSkipsDisabled: a disabled schedule never shows as due even if
// its NextRunAt is in the past.
func TestScheduleDueSkipsDisabled(t *testing.T) {
	ctx := context.Background()
	s := newScheduleStore(t)
	past := time.Now().Add(-time.Hour)
	if _, err := s.Create(ctx, schedule.Schedule{Instructions: "p", Cron: "@daily", Enabled: false, NextRunAt: past}); err != nil {
		t.Fatal(err)
	}
	due, err := s.Due(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("due = %+v, want none (disabled)", due)
	}
}

// TestScheduleUnacknowledgedOccurrenceSurvivesStoreReopen covers the durable
// half of worker restart semantics. An occurrence that was never claimed must
// remain discoverable through a fresh store after process restart.
func TestScheduleUnacknowledgedOccurrenceSurvivesStoreReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "scopeapp.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("open initial store: %v", err)
	}
	store := sqlite.NewScheduleStore(db)
	past := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	created, err := store.Create(ctx, schedule.Schedule{
		Instructions: "p", Cron: "@daily", Enabled: true, NextRunAt: past,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatalf("close initial store: %v", closeErr)
	}

	reopenedDB, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { _ = reopenedDB.Close() })
	due, err := sqlite.NewScheduleStore(reopenedDB).Due(ctx, time.Now(), 100)
	if err != nil {
		t.Fatalf("due after reopen: %v", err)
	}
	if len(due) != 1 || due[0].ID != created.ID {
		t.Fatalf("due after reopen = %+v, want unacknowledged %q", due, created.ID)
	}
}

func TestScheduleQueriesUseIDAsStableTieBreaker(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewScheduleStore(db)
	createdAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC).UnixMilli()
	nextRunAt := time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC).UnixMilli()
	for _, id := range []string{"sch_a", "sch_c", "sch_b"} {
		_, execContextErr := db.ExecContext(t.Context(), `INSERT INTO schedules(
			id, title, instructions, cwd, provider, model, cron, enabled,
			last_run_at, next_run_at, created_at, revision
		) VALUES (?, '', 'review', '', '', '', '0 9 * * *', 1, 0, ?, ?, 1)`,
			id, nextRunAt, createdAt)
		if execContextErr != nil {
			t.Fatalf("insert %s: %v", id, execContextErr)
		}
	}

	listed, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	due, err := store.Due(t.Context(), time.UnixMilli(nextRunAt), 100)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	ids := func(items []schedule.Schedule) []string {
		out := make([]string, len(items))
		for i := range items {
			out[i] = items[i].ID
		}
		return out
	}
	if got, want := ids(listed), []string{"sch_c", "sch_b", "sch_a"}; !slices.Equal(got, want) {
		t.Fatalf("List IDs = %v, want %v", got, want)
	}
	firstPage, err := store.ListPage(t.Context(), time.Time{}, "", 2)
	if err != nil {
		t.Fatalf("ListPage first: %v", err)
	}
	if got, want := ids(firstPage), []string{"sch_c", "sch_b"}; !slices.Equal(got, want) {
		t.Fatalf("ListPage first IDs = %v, want %v", got, want)
	}
	secondPage, err := store.ListPage(t.Context(), firstPage[1].CreatedAt, firstPage[1].ID, 2)
	if err != nil {
		t.Fatalf("ListPage second: %v", err)
	}
	if got, want := ids(secondPage), []string{"sch_a"}; !slices.Equal(got, want) {
		t.Fatalf("ListPage second IDs = %v, want %v", got, want)
	}
	if got, want := ids(due), []string{"sch_a", "sch_b", "sch_c"}; !slices.Equal(got, want) {
		t.Fatalf("Due IDs = %v, want %v", got, want)
	}
}

func TestScheduleDuePrioritizesOldestBacklog(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "scopeapp.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewScheduleStore(db)
	ctx := t.Context()
	oldDueAt := time.Date(2026, 7, 19, 10, 0, 0, 0, time.UTC)
	newDueAt := oldDueAt.Add(time.Minute)
	insert := func(id string, dueAt time.Time) {
		t.Helper()
		_, execContextErr := db.ExecContext(ctx, `INSERT INTO schedules(
			id, title, instructions, cwd, provider, model, cron, enabled,
			last_run_at, next_run_at, created_at, revision
		) VALUES (?, '', 'review', '', '', '', '0 9 * * *', 1, 0, ?, ?, 1)`,
			id, dueAt.UnixMilli(), dueAt.UnixMilli())
		if execContextErr != nil {
			t.Fatalf("insert %s: %v", id, execContextErr)
		}
	}
	insert("sch_old", oldDueAt)
	for index := range 32 {
		insert(fmt.Sprintf("sch_new_%02d", index), newDueAt)
	}

	due, err := store.Due(ctx, newDueAt, 32)
	if err != nil {
		t.Fatalf("Due: %v", err)
	}
	if len(due) != 32 {
		t.Fatalf("Due count = %d, want 32", len(due))
	}
	if due[0].ID != "sch_old" {
		t.Fatalf("first due ID = %q, want oldest backlog sch_old", due[0].ID)
	}
	for _, item := range due[1:] {
		if item.ID == "sch_old" {
			t.Fatalf("oldest backlog appeared more than once: %+v", due)
		}
	}
}
