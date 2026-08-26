package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/modelref"
	"github.com/Tangerg/lynx/app/runtime/internal/domain/schedule"
)

// ScheduleStore is the SQLite persistence adapter for scheduled runs. The DB
// must have been opened via [Open] so the schedules table exists.
type ScheduleStore struct {
	db *sql.DB
}

// NewScheduleStore wires the given *sql.DB to the schedule persistence surface.
func NewScheduleStore(db *sql.DB) *ScheduleStore {
	return &ScheduleStore{db: db}
}

func (s *ScheduleStore) Create(ctx context.Context, sc schedule.Schedule) (schedule.Schedule, error) {
	if err := sc.Validate(); err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: validate schedule: %w", err)
	}
	sc.ID = schedule.IDPrefix + uuid.NewString()
	sc.CreatedAt = time.Now().UTC()
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO schedules (id, title, instructions, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
		sc.ID, sc.Title, sc.Instructions, sc.CWD, sc.ModelSelection.Provider(), sc.ModelSelection.Model(), sc.Cron,
		boolToInt(sc.Enabled), toMillis(sc.LastRunAt), toMillis(sc.NextRunAt), sc.CreatedAt.UnixMilli())
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: create schedule: %w", err)
	}
	sc.Revision = 1
	return sc, nil
}

func (s *ScheduleStore) Update(ctx context.Context, sc schedule.Schedule, expectedRevision uint64) (schedule.Schedule, error) {
	if err := sc.Validate(); err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: validate schedule: %w", err)
	}
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE schedules
		 SET title = ?, instructions = ?, cwd = ?, provider = ?, model = ?, cron = ?, enabled = ?, next_run_at = ?, revision = revision + 1
		 WHERE id = ? AND revision = ?`,
		sc.Title, sc.Instructions, sc.CWD, sc.ModelSelection.Provider(), sc.ModelSelection.Model(), sc.Cron,
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
		`SELECT id, title, instructions, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules WHERE id = ?`, id)
	sc, err := scanSchedule(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: get schedule: %w", err)
	}
	if err := sc.Validate(); err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: validate schedule: %w", err)
	}
	return sc, nil
}

func (s *ScheduleStore) List(ctx context.Context) ([]schedule.Schedule, error) {
	return s.ListPage(ctx, time.Time{}, "", 0)
}

// ListPage returns schedules newest-created first, bounded by the query. after is
// the (creation time, id) position a previous page ended at; the id breaks ties,
// so two schedules created in the same nanosecond cannot be dropped or repeated
// across a page boundary.
func (s *ScheduleStore) ListPage(ctx context.Context, afterCreatedAt time.Time, afterID string, limit int) ([]schedule.Schedule, error) {
	query := `SELECT id, title, instructions, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules`
	var args []any
	if !afterCreatedAt.IsZero() || afterID != "" {
		query += ` WHERE created_at < ? OR (created_at = ? AND id < ?)`
		afterMillis := afterCreatedAt.UnixMilli()
		args = append(args, afterMillis, afterMillis, afterID)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	return s.query(ctx, "list schedules", query, args...)
}

func (s *ScheduleStore) Due(ctx context.Context, now time.Time, limit int) ([]schedule.Schedule, error) {
	if limit <= 0 {
		return nil, nil
	}
	return s.query(ctx, "list due schedules",
		`SELECT id, title, instructions, cwd, provider, model, cron, enabled, last_run_at, next_run_at, created_at, revision
		 FROM schedules
		 WHERE enabled = 1 AND next_run_at > 0 AND next_run_at <= ?
		 ORDER BY next_run_at, id
		 LIMIT ?`, now.UnixMilli(), limit)
}

// Claim atomically advances a due schedule's cursor and materializes its
// immutable occurrence. The pending row is the durable work item a future
// worker dispatches after a process crash; cursor advancement therefore cannot
// produce either a duplicate accepted run or a silently lost occurrence.
// LastRunAt remains an accepted-Run fact and is intentionally not touched here.
func (s *ScheduleStore) Claim(ctx context.Context, occurrence schedule.Occurrence) (claimed bool, err error) {
	if occurrence.ID == "" || occurrence.Schedule.ID == "" || occurrence.SessionID == "" || occurrence.RunID == "" {
		return false, errors.New("sqlite: schedule occurrence identity is required")
	}
	err = RunInTx(ctx, s.db, func(ctx context.Context) error {
		res, execContextErr := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE schedules SET next_run_at = ?, revision = revision + 1
				 WHERE id = ? AND revision = ? AND next_run_at = ?
				   AND NOT EXISTS (
						SELECT 1 FROM schedule_firings
						 WHERE schedule_id = ? AND state = 'pending'
				   )`,
			toMillis(occurrence.NextRunAt), occurrence.Schedule.ID, occurrence.Schedule.Revision,
			toMillis(occurrence.DueAt), occurrence.Schedule.ID)
		if execContextErr != nil {
			return fmt.Errorf("sqlite: claim schedule occurrence: %w", execContextErr)
		}
		changed, execContextErr := res.RowsAffected()
		if execContextErr != nil {
			return fmt.Errorf("sqlite: inspect schedule occurrence claim: %w", execContextErr)
		}
		if changed == 0 {
			return nil
		}
		_, execContextErr = conn(ctx, s.db).ExecContext(ctx,
			`INSERT INTO schedule_firings(
				id, schedule_id, title, instructions, cwd, provider, model, cron,
				due_at, fired_at, next_run_at, session_id, run_id, state
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
			occurrence.ID, occurrence.Schedule.ID, occurrence.Schedule.Title, occurrence.Schedule.Instructions,
			occurrence.Schedule.CWD, occurrence.Schedule.ModelSelection.Provider(), occurrence.Schedule.ModelSelection.Model(), occurrence.Schedule.Cron,
			toMillis(occurrence.DueAt), toMillis(occurrence.FiredAt), toMillis(occurrence.NextRunAt), occurrence.SessionID, occurrence.RunID)
		if execContextErr != nil {
			return fmt.Errorf("sqlite: persist schedule occurrence: %w", execContextErr)
		}
		claimed = true
		return nil
	})
	return claimed, err
}

// Pending lists durable occurrences whose Run opening has not committed. They
// carry a full schedule snapshot, so later schedule edits or deletion cannot
// rewrite work that was already due.
func (s *ScheduleStore) Pending(ctx context.Context, limit int) ([]schedule.Occurrence, error) {
	if limit <= 0 {
		return nil, nil
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
		`SELECT id, schedule_id, title, instructions, cwd, provider, model, cron,
			due_at, fired_at, next_run_at, session_id, run_id
		 FROM schedule_firings WHERE state = 'pending' ORDER BY due_at, id
		 LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list pending schedule occurrences: %w", err)
	}
	defer rows.Close()
	var occurrences []schedule.Occurrence
	for rows.Next() {
		occurrence, err := scanOccurrence(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan pending schedule occurrence: %w", err)
		}
		occurrences = append(occurrences, occurrence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list pending schedule occurrences: %w", err)
	}
	return occurrences, nil
}

// Accept confirms the occurrence in the same transaction as its Run opening.
// Repeating the same confirmation is harmless; any other run id is a durable
// ownership violation rather than an invitation to create a duplicate run.
func (s *ScheduleStore) Accept(ctx context.Context, occurrenceID, runID string) error {
	res, err := conn(ctx, s.db).ExecContext(ctx,
		`UPDATE schedule_firings SET state = 'accepted' WHERE id = ? AND run_id = ? AND state = 'pending'`,
		occurrenceID, runID)
	if err != nil {
		return fmt.Errorf("sqlite: accept schedule occurrence: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect schedule occurrence acceptance: %w", err)
	}
	if changed != 0 {
		var scheduleID string
		var firedAt int64
		if scanErr := conn(ctx, s.db).QueryRowContext(ctx,
			`SELECT schedule_id, fired_at FROM schedule_firings WHERE id = ? AND run_id = ?`, occurrenceID, runID).Scan(&scheduleID, &firedAt); scanErr != nil {
			return fmt.Errorf("sqlite: load accepted schedule occurrence: %w", scanErr)
		}
		if _, execContextErr := conn(ctx, s.db).ExecContext(ctx,
			`UPDATE schedules
			 SET last_run_at = MAX(last_run_at, ?), revision = revision + 1
			 WHERE id = ?`, firedAt, scheduleID); execContextErr != nil {
			return fmt.Errorf("sqlite: record accepted schedule occurrence: %w", execContextErr)
		}
		return nil
	}
	var storedRunID, state string
	err = conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT run_id, state FROM schedule_firings WHERE id = ?`, occurrenceID).Scan(&storedRunID, &state)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("sqlite: schedule occurrence not found")
	}
	if err != nil {
		return fmt.Errorf("sqlite: inspect schedule occurrence acceptance: %w", err)
	}
	if storedRunID == runID && state == "accepted" {
		return nil
	}
	return errors.New("sqlite: schedule occurrence is owned by another run")
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

func (s *ScheduleStore) Delete(ctx context.Context, id string) (bool, error) {
	result, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("sqlite: delete schedule: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect deleted schedule: %w", err)
	}
	return deleted > 0, nil
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
	var provider, model string
	var enabled, lastMillis, nextMillis, createdMillis int64
	if err := scan(&sc.ID, &sc.Title, &sc.Instructions, &sc.CWD, &provider, &model, &sc.Cron,
		&enabled, &lastMillis, &nextMillis, &createdMillis, &sc.Revision); err != nil {
		return schedule.Schedule{}, err
	}
	selection, err := modelref.New(provider, model)
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: decode schedule model selection: %w", err)
	}
	sc.ModelSelection = selection
	sc.Enabled = enabled != 0
	sc.LastRunAt = fromMillis(lastMillis)
	sc.NextRunAt = fromMillis(nextMillis)
	sc.CreatedAt = time.UnixMilli(createdMillis).UTC()
	return sc, nil
}

func scanOccurrence(scan func(...any) error) (schedule.Occurrence, error) {
	var occurrence schedule.Occurrence
	var provider, model string
	var dueAt, firedAt, nextRunAt int64
	if err := scan(&occurrence.ID, &occurrence.Schedule.ID, &occurrence.Schedule.Title, &occurrence.Schedule.Instructions,
		&occurrence.Schedule.CWD, &provider, &model, &occurrence.Schedule.Cron,
		&dueAt, &firedAt, &nextRunAt, &occurrence.SessionID, &occurrence.RunID); err != nil {
		return schedule.Occurrence{}, err
	}
	selection, err := modelref.New(provider, model)
	if err != nil {
		return schedule.Occurrence{}, fmt.Errorf("decode schedule occurrence model selection: %w", err)
	}
	occurrence.Schedule.ModelSelection = selection
	occurrence.DueAt = fromMillis(dueAt)
	occurrence.FiredAt = fromMillis(firedAt)
	occurrence.NextRunAt = fromMillis(nextRunAt)
	return occurrence, nil
}
