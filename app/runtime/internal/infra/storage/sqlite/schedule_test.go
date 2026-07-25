package sqlite_test

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
	"github.com/Tangerg/lynx/app/runtime/internal/infra/storage/sqlite"
)

func newScheduleStore(t *testing.T) *sqlite.ScheduleStore {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return sqlite.NewScheduleStore(db)
}

// TestScheduleCRUD covers create (id assigned, persisted verbatim), get, the
// next-due query, mark-fired, update, and delete.
func TestScheduleCRUD(t *testing.T) {
	ctx := context.Background()
	s := newScheduleStore(t)

	past := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	created, err := s.Create(ctx, schedule.Schedule{
		Title: "standup", Prompt: "summarize the diff", Cwd: "/proj",
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
	if got.Prompt != "summarize the diff" || got.Cron != "0 9 * * 1-5" || !got.Enabled {
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

	// MarkFired records lastRunAt + advances nextRunAt to the future → no longer due.
	now := time.Now().UTC().Truncate(time.Millisecond)
	future := now.Add(24 * time.Hour)
	if err := s.MarkFired(ctx, created.ID, now, created.NextRunAt, future); err != nil {
		t.Fatalf("markFired: %v", err)
	}
	due, _ = s.Due(ctx, time.Now(), 100)
	if len(due) != 0 {
		t.Errorf("due after markFired = %+v, want none", due)
	}
	got, _ = s.Get(ctx, created.ID)
	if !got.LastRunAt.Equal(now) {
		t.Errorf("LastRunAt = %v, want %v", got.LastRunAt, now)
	}

	// Update: disabling clears the due index (NextRunAt zero) → never due.
	got.Enabled = false
	got.NextRunAt = time.Time{}
	got.Title = "renamed"
	if _, err := s.Update(ctx, got, got.Revision); err != nil {
		t.Fatalf("update: %v", err)
	}
	reread, _ := s.Get(ctx, created.ID)
	if reread.Enabled || reread.Title != "renamed" || !reread.NextRunAt.IsZero() {
		t.Errorf("update not applied: %+v", reread)
	}

	if err := s.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, created.ID); err != schedule.ErrNotFound {
		t.Errorf("get after delete err = %v, want ErrNotFound", err)
	}
}

// TestScheduleRecordRunLeavesCursor: a manual run-now (RecordRun) updates
// LastRunAt but must NOT touch NextRunAt. Re-stamping a cursor value read before
// the worker advanced it would rewind the schedule and re-fire it every tick —
// the bug RecordRun (vs MarkFired) exists to prevent.
func TestScheduleRecordRunLeavesCursor(t *testing.T) {
	ctx := context.Background()
	s := newScheduleStore(t)

	past := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	created, err := s.Create(ctx, schedule.Schedule{
		Prompt: "p", Cron: "@daily", Enabled: true, NextRunAt: past,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The worker fires and advances the cursor into the future → no longer due.
	future := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond)
	if err := s.MarkFired(ctx, created.ID, time.Now().UTC(), created.NextRunAt, future); err != nil {
		t.Fatalf("markFired: %v", err)
	}

	// A manual run-now lands afterwards. It must leave the advanced cursor alone.
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

func TestScheduleOccurrenceSurvivesDispatchAndAcceptsOnce(t *testing.T) {
	ctx := t.Context()
	store := newScheduleStore(t)
	dueAt := time.Date(2026, 7, 25, 9, 0, 0, 0, time.UTC)
	nextAt := dueAt.Add(time.Hour)
	created, err := store.Create(ctx, schedule.Schedule{
		Title: "hourly", Prompt: "review", Cron: "0 * * * *", Enabled: true, NextRunAt: dueAt,
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
	if err := store.Accept(ctx, occurrence.ID, occurrence.RunID); err != nil {
		t.Fatalf("Accept: %v", err)
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
		Title: "hourly", Prompt: "review", Cron: "0 * * * *", Enabled: true, NextRunAt: firstDueAt,
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

	// The worker can observe the later cron slot while this first occurrence is
	// still waiting for Run admission. It must not advance the cursor again or
	// materialize a second recovery item for the same schedule.
	second := schedule.Occurrence{
		ID: created.ID + ":second", Schedule: created,
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
	got, err := store.Get(ctx, created.ID)
	if err != nil || !got.NextRunAt.Equal(secondDueAt) {
		t.Fatalf("schedule cursor = (%v, %v), want %v", got.NextRunAt, err, secondDueAt)
	}

	if err := store.Accept(ctx, first.ID, first.RunID); err != nil {
		t.Fatalf("Accept first occurrence: %v", err)
	}
	claimed, err = store.Claim(ctx, second)
	if err != nil || !claimed {
		t.Fatalf("second Claim after accept = (%v, %v), want true, nil", claimed, err)
	}
}

func TestScheduleStoreRejectsDuplicatePendingRows(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewScheduleStore(db)
	dueAt := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	created, err := store.Create(ctx, schedule.Schedule{
		Prompt: "review", Cron: "@hourly", Enabled: true, NextRunAt: dueAt,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	insert := func(id, sessionID, runID string) error {
		_, err := db.ExecContext(ctx, `INSERT INTO schedule_firings(
			id, schedule_id, prompt, cron, due_at, fired_at, next_run_at, session_id, run_id, state
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

// TestScheduleMarkFiredCASLosesToReschedule: MarkFired advances the cursor only
// if next_run_at is still what the worker saw at Due time. A concurrent
// schedules.Update that rescheduled (new cron → new next_run_at) between the
// worker's Due read and its MarkFired write must WIN — the worker must not
// clobber the new cursor with a value computed from the stale cron. The firing
// is still recorded (last_run_at) so the run isn't lost.
func TestScheduleMarkFiredCASLosesToReschedule(t *testing.T) {
	ctx := context.Background()
	s := newScheduleStore(t)

	past := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	created, err := s.Create(ctx, schedule.Schedule{Prompt: "p", Cron: "@daily", Enabled: true, NextRunAt: past})
	if err != nil {
		t.Fatal(err)
	}

	// A user reschedules (new cron) between the worker's Due read (which saw
	// `past`) and its MarkFired write: next_run_at is now `rescheduled`.
	rescheduled := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Millisecond)
	got, _ := s.Get(ctx, created.ID)
	got.NextRunAt = rescheduled
	if _, err := s.Update(ctx, got, got.Revision); err != nil {
		t.Fatalf("update: %v", err)
	}

	// The worker now fires with the STALE prev (`past`) + a stale-cron next. The
	// CAS must miss: the rescheduled cursor stays, but last_run_at is recorded.
	ranAt := time.Now().UTC().Truncate(time.Millisecond)
	staleNext := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Millisecond)
	if err := s.MarkFired(ctx, created.ID, ranAt, past, staleNext); err != nil {
		t.Fatalf("markFired: %v", err)
	}

	reread, _ := s.Get(ctx, created.ID)
	if !reread.NextRunAt.Equal(rescheduled) {
		t.Errorf("NextRunAt = %v, want %v (reschedule must win the stale advance)", reread.NextRunAt, rescheduled)
	}
	if !reread.LastRunAt.Equal(ranAt) {
		t.Errorf("LastRunAt = %v, want %v (the firing must still be recorded)", reread.LastRunAt, ranAt)
	}
}

// TestScheduleUpdateNotFound: updating an unknown id reports ErrNotFound.
func TestScheduleUpdateNotFound(t *testing.T) {
	s := newScheduleStore(t)
	_, err := s.Update(context.Background(), schedule.Schedule{ID: "sch_nope", Prompt: "x", Cron: "@daily"}, 1)
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
	if _, err := s.Create(ctx, schedule.Schedule{Prompt: "p", Cron: "@daily", Enabled: false, NextRunAt: past}); err != nil {
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
// half of worker restart semantics. The worker only calls MarkFired after the
// application admits a Run; when it cannot, this unchanged due row must still
// be discoverable through a fresh store after process restart.
func TestScheduleUnacknowledgedOccurrenceSurvivesStoreReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lyra.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("open initial store: %v", err)
	}
	store := sqlite.NewScheduleStore(db)
	past := time.Now().Add(-time.Hour).UTC().Truncate(time.Millisecond)
	created, err := store.Create(ctx, schedule.Schedule{
		Prompt: "p", Cron: "@daily", Enabled: true, NextRunAt: past,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close initial store: %v", err)
	}

	reopenedDB, err := sqlite.Open(path)
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
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewScheduleStore(db)
	createdAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC).UnixMilli()
	nextRunAt := time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC).UnixMilli()
	for _, id := range []string{"sch_a", "sch_c", "sch_b"} {
		_, err := db.ExecContext(t.Context(), `INSERT INTO schedules(
			id, title, prompt, cwd, provider, model, cron, enabled,
			last_run_at, next_run_at, created_at, revision
		) VALUES (?, '', 'review', '', '', '', '0 9 * * *', 1, 0, ?, ?, 1)`,
			id, nextRunAt, createdAt)
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
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
	if got, want := ids(due), []string{"sch_a", "sch_b", "sch_c"}; !slices.Equal(got, want) {
		t.Fatalf("Due IDs = %v, want %v", got, want)
	}
}

func TestScheduleDuePrioritizesOldestBacklog(t *testing.T) {
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "lyra.db"))
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
		_, err := db.ExecContext(ctx, `INSERT INTO schedules(
			id, title, prompt, cwd, provider, model, cron, enabled,
			last_run_at, next_run_at, created_at, revision
		) VALUES (?, '', 'review', '', '', '', '0 9 * * *', 1, 0, ?, ?, 1)`,
			id, dueAt.UnixMilli(), dueAt.UnixMilli())
		if err != nil {
			t.Fatalf("insert %s: %v", id, err)
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
