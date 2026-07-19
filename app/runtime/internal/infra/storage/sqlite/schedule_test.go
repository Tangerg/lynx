package sqlite_test

import (
	"context"
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
	due, err := s.Due(ctx, time.Now())
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
	due, _ = s.Due(ctx, time.Now())
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
	due, _ := s.Due(ctx, time.Now())
	if len(due) != 0 {
		t.Errorf("due after RecordRun = %+v, want none (cursor still in the future)", due)
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
	due, err := s.Due(ctx, time.Now())
	if err != nil {
		t.Fatalf("due: %v", err)
	}
	if len(due) != 0 {
		t.Errorf("due = %+v, want none (disabled)", due)
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
	due, err := store.Due(t.Context(), time.UnixMilli(nextRunAt))
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
	want := []string{"sch_c", "sch_b", "sch_a"}
	if got := ids(listed); !slices.Equal(got, want) {
		t.Fatalf("List IDs = %v, want %v", got, want)
	}
	if got := ids(due); !slices.Equal(got, want) {
		t.Fatalf("Due IDs = %v, want %v", got, want)
	}
}
