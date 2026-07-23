package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// ScheduleStore is the SQLite persistence adapter for scheduled runs — the persistent home
// for scheduled runs. The DB must have been opened via [Open] so the schedules
// table exists.
type ScheduleStore struct {
	db *sql.DB
}

// NewScheduleStore wires the given *sql.DB to the schedule persistence surface.
func NewScheduleStore(db *sql.DB) *ScheduleStore {
	return &ScheduleStore{db: db}
}

func (s *ScheduleStore) Create(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	sc.ID = schedule.IDPrefix + uuid.NewString()
	sc.CreatedAt = time.Now().UTC()
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO schedules (id, title, prompt, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		sc.ID, sc.Title, sc.Prompt, sc.Cwd, sc.Provider, sc.Model, sc.Cron,
		boolToInt(sc.Enabled), toMillis(sc.LastRunAt), toMillis(sc.NextRunAt), sc.CreatedAt.UnixMilli())
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: create schedule: %w", err)
	}
	sc.Revision = 1
	return sc, nil
}

func (s *ScheduleStore) Update(ctx context.Context, sc schedule.Schedule, expectedRevision uint64) (schedule.Schedule, error) {
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE schedules
		 SET title = ?, prompt = ?, cwd = ?, provider = ?, model = ?, cron = ?, enabled = ?, next_run_at = ?, revision = revision + 1
		 WHERE id = ? AND revision = ?`,
		sc.Title, sc.Prompt, sc.Cwd, sc.Provider, sc.Model, sc.Cron,
		boolToInt(sc.Enabled), toMillis(sc.NextRunAt), sc.ID, expectedRevision)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: update schedule: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: inspect schedule update: %w", err)
	}
	if changed == 0 {
		if _, getErr := s.Get(ctx, sc.ID); getErr != nil {
			return schedule.Schedule{}, getErr
		}
		return schedule.Schedule{}, schedule.ErrRevisionConflict
	}
	return s.Get(ctx, sc.ID)
}

func (s *ScheduleStore) Get(ctx context.Context, id string) (schedule.Schedule, error) {
	row := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT id, title, prompt, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules WHERE id = ?`, id)
	sc, err := scanSchedule(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: get schedule: %w", err)
	}
	return sc, nil
}

func (s *ScheduleStore) List(ctx context.Context) ([]schedule.Schedule, error) {
	return s.query(ctx, "list schedules",
		`SELECT id, title, prompt, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules ORDER BY created_at DESC, id DESC`)
}

func (s *ScheduleStore) Due(ctx context.Context, now time.Time) ([]schedule.Schedule, error) {
	return s.query(ctx, "list due schedules",
		`SELECT id, title, prompt, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules
		 WHERE enabled = 1 AND next_run_at > 0 AND next_run_at <= ?
		 ORDER BY next_run_at DESC, id DESC`, now.UnixMilli())
}

func (s *ScheduleStore) MarkFired(ctx context.Context, id string, ranAt, prevNextRunAt, nextRunAt time.Time) error {
	// CAS the cursor: advance next_run_at only if it's still the value the worker
	// saw at Due time. A concurrent schedules.Update that rescheduled (new cron →
	// new next_run_at) between that read and now must win, not be overwritten with
	// a value computed from the stale cron. If the guard misses (rescheduled, or
	// the row was deleted), the run still fired — record last_run_at without
	// rewinding the cursor.
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE schedules SET last_run_at = ?, next_run_at = ?, revision = revision + 1 WHERE id = ? AND next_run_at = ?`,
		toMillis(ranAt), toMillis(nextRunAt), id, toMillis(prevNextRunAt))
	if err != nil {
		return fmt.Errorf("sqlite: mark schedule fired: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect mark schedule fired: %w", err)
	}
	if changed == 0 {
		return s.RecordRun(ctx, id, ranAt)
	}
	return nil
}

// RecordRun moves only last_run_at; next_run_at is left as-is so a manual
// run-now never rewinds the cron cursor.
func (s *ScheduleStore) RecordRun(ctx context.Context, id string, ranAt time.Time) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE schedules SET last_run_at = ?, revision = revision + 1 WHERE id = ?`,
		toMillis(ranAt), id); err != nil {
		return fmt.Errorf("sqlite: record schedule run: %w", err)
	}
	return nil
}

func (s *ScheduleStore) Delete(ctx context.Context, id string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id); err != nil {
		return fmt.Errorf("sqlite: delete schedule: %w", err)
	}
	return nil
}

func (s *ScheduleStore) query(ctx context.Context, operation, q string, args ...any) ([]schedule.Schedule, error) {
	rows, err := conn(ctx, s.db).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	defer rows.Close()
	var out []schedule.Schedule
	for rows.Next() {
		sc, err := scanSchedule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan schedule: %w", err)
		}
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: %s: %w", operation, err)
	}
	return out, nil
}

// scanSchedule decodes one row via the given Scan func (sql.Row or sql.Rows
// share the signature), converting the int-millis time columns back to
// time.Time (0 ⇒ zero time).
func scanSchedule(scan func(...any) error) (schedule.Schedule, error) {
	var sc schedule.Schedule
	var enabled, lastMs, nextMs, createdMs int64
	if err := scan(&sc.ID, &sc.Title, &sc.Prompt, &sc.Cwd, &sc.Provider, &sc.Model, &sc.Cron,
		&enabled, &lastMs, &nextMs, &createdMs, &sc.Revision); err != nil {
		return schedule.Schedule{}, err
	}
	sc.Enabled = enabled != 0
	sc.LastRunAt = fromMillis(lastMs)
	sc.NextRunAt = fromMillis(nextMs)
	sc.CreatedAt = time.UnixMilli(createdMs).UTC()
	return sc, nil
}
