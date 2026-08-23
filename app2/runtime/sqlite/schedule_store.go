package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/schedule"
)

func (database *Database) CreateSchedule(ctx context.Context, value schedule.Schedule) error {
	state := value.State()
	_, err := database.database.ExecContext(ctx, `
		INSERT INTO schedules (
			id, title, instructions, workspace_path, provider, model, cron, enabled,
			last_run_at, next_run_at, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		state.ID, state.Title, state.Instructions, state.Workspace, state.Provider, state.Model,
		state.Cron, state.Enabled, encodeScheduleTime(state.LastRunAt), encodeScheduleTime(state.NextRunAt),
		state.Revision, encodeTime(state.CreatedAt), encodeTime(state.UpdatedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite: create schedule %q: %w", state.ID, err)
	}
	return nil
}

func (database *Database) GetSchedule(ctx context.Context, id string) (schedule.Schedule, error) {
	value, err := scanSchedule(database.database.QueryRowContext(ctx, scheduleSelect+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return schedule.Schedule{}, schedule.ErrNotFound
	}
	if err != nil {
		return schedule.Schedule{}, fmt.Errorf("sqlite: get schedule %q: %w", id, err)
	}
	return value, nil
}

func (database *Database) ListSchedulePage(
	ctx context.Context,
	limit int,
	after *schedule.Cursor,
) (schedule.Page, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	query := scheduleSelect
	arguments := make([]any, 0, 4)
	if after != nil {
		query += ` WHERE created_at < ? OR (created_at = ? AND id < ?)`
		createdAt := encodeTime(after.CreatedAt)
		arguments = append(arguments, createdAt, createdAt, after.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	arguments = append(arguments, limit+1)
	rows, err := database.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return schedule.Page{}, fmt.Errorf("sqlite: list schedules: %w", err)
	}
	defer rows.Close()
	values := make([]schedule.Schedule, 0, limit+1)
	for rows.Next() {
		value, err := scanSchedule(rows.Scan)
		if err != nil {
			return schedule.Page{}, fmt.Errorf("sqlite: scan schedule: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return schedule.Page{}, fmt.Errorf("sqlite: iterate schedules: %w", err)
	}
	page := schedule.Page{Schedules: values}
	if len(values) > limit {
		last := values[limit-1]
		page.Schedules = values[:limit]
		page.Next = &schedule.Cursor{CreatedAt: last.CreatedAt(), ID: last.ID()}
	}
	return page, nil
}

func (database *Database) UpdateSchedule(
	ctx context.Context,
	value schedule.Schedule,
	expectedRevision uint64,
) error {
	state := value.State()
	result, err := database.database.ExecContext(ctx, `
		UPDATE schedules SET
			title = ?, instructions = ?, workspace_path = ?, provider = ?, model = ?,
			cron = ?, enabled = ?, last_run_at = ?, next_run_at = ?, revision = ?, updated_at = ?
		WHERE id = ? AND revision = ?`,
		state.Title, state.Instructions, state.Workspace, state.Provider, state.Model,
		state.Cron, state.Enabled, encodeScheduleTime(state.LastRunAt), encodeScheduleTime(state.NextRunAt),
		state.Revision, encodeTime(state.UpdatedAt), state.ID, expectedRevision,
	)
	if err != nil {
		return fmt.Errorf("sqlite: update schedule %q: %w", state.ID, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect schedule update %q: %w", state.ID, err)
	}
	if changed == 1 {
		return nil
	}
	if _, err := database.GetSchedule(ctx, state.ID); errors.Is(err, schedule.ErrNotFound) {
		return schedule.ErrNotFound
	} else if err != nil {
		return err
	}
	return schedule.ErrRevisionConflict
}

func (database *Database) DeleteSchedule(ctx context.Context, id string) (bool, error) {
	result, err := database.database.ExecContext(ctx, `DELETE FROM schedules WHERE id = ?`, id)
	if err != nil {
		return false, fmt.Errorf("sqlite: delete schedule %q: %w", id, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect schedule deletion %q: %w", id, err)
	}
	return changed > 0, nil
}

func (database *Database) DueSchedules(ctx context.Context, now time.Time, limit int) ([]schedule.Schedule, error) {
	if limit <= 0 {
		return []schedule.Schedule{}, nil
	}
	rows, err := database.database.QueryContext(ctx, scheduleSelect+`
		WHERE enabled = 1 AND next_run_at <= ?
		ORDER BY next_run_at, id LIMIT ?`, encodeTime(now), limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list due schedules: %w", err)
	}
	defer rows.Close()
	values := make([]schedule.Schedule, 0, limit)
	for rows.Next() {
		value, err := scanSchedule(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan due schedule: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate due schedules: %w", err)
	}
	return values, nil
}

func (database *Database) ClaimScheduleOccurrence(
	ctx context.Context,
	occurrence schedule.Occurrence,
) (bool, error) {
	claimed, err := occurrence.ClaimedSchedule()
	if err != nil {
		return false, err
	}
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("sqlite: begin schedule occurrence claim: %w", err)
	}
	defer transaction.Rollback()
	result, err := transaction.ExecContext(ctx, `
		UPDATE schedules SET next_run_at = ?, revision = ?, updated_at = ?
		WHERE id = ? AND revision = ? AND next_run_at = ?
			AND NOT EXISTS (
				SELECT 1 FROM schedule_occurrences
				WHERE schedule_id = ? AND status = 'pending'
			)`,
		encodeTime(claimed.NextRunAt()), claimed.Revision(), encodeTime(claimed.UpdatedAt()),
		claimed.ID(), occurrence.Schedule().Revision(), encodeTime(occurrence.DueAt()), claimed.ID(),
	)
	if err != nil {
		return false, fmt.Errorf("sqlite: claim schedule occurrence %q: %w", occurrence.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect schedule occurrence claim %q: %w", occurrence.ID(), err)
	}
	if changed == 0 {
		return false, nil
	}
	if err := insertScheduleOccurrence(ctx, transaction, occurrence); err != nil {
		return false, err
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("sqlite: commit schedule occurrence claim %q: %w", occurrence.ID(), err)
	}
	return true, nil
}

func (database *Database) PendingScheduleOccurrences(
	ctx context.Context,
	limit int,
) ([]schedule.Occurrence, error) {
	if limit <= 0 {
		return []schedule.Occurrence{}, nil
	}
	rows, err := database.database.QueryContext(ctx, occurrenceSelect+`
		WHERE status = 'pending' ORDER BY due_at, id LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list pending schedule occurrences: %w", err)
	}
	defer rows.Close()
	values := make([]schedule.Occurrence, 0, limit)
	for rows.Next() {
		value, err := scanScheduleOccurrence(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("sqlite: scan pending schedule occurrence: %w", err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate pending schedule occurrences: %w", err)
	}
	return values, nil
}

func (database *Database) PruneAcceptedScheduleOccurrences(
	ctx context.Context,
	before time.Time,
	limit int,
) error {
	if limit <= 0 {
		return nil
	}
	_, err := database.database.ExecContext(ctx, `
		DELETE FROM schedule_occurrences WHERE id IN (
			SELECT id FROM schedule_occurrences
			WHERE status = 'accepted' AND accepted_at < ?
			ORDER BY accepted_at, id LIMIT ?
		)`, encodeTime(before), limit)
	if err != nil {
		return fmt.Errorf("sqlite: prune accepted schedule occurrences: %w", err)
	}
	return nil
}

const scheduleSelect = `
	SELECT id, title, instructions, workspace_path, provider, model, cron, enabled,
		last_run_at, next_run_at, revision, created_at, updated_at
	FROM schedules `

func scanSchedule(scan func(...any) error) (schedule.Schedule, error) {
	var state schedule.State
	var enabled int
	var lastRunAt string
	var nextRunAt string
	var createdAt string
	var updatedAt string
	if err := scan(
		&state.ID, &state.Title, &state.Instructions, &state.Workspace, &state.Provider, &state.Model,
		&state.Cron, &enabled, &lastRunAt, &nextRunAt, &state.Revision, &createdAt, &updatedAt,
	); err != nil {
		return schedule.Schedule{}, err
	}
	state.Enabled = enabled == 1
	var err error
	state.LastRunAt, err = decodeScheduleTime(lastRunAt)
	if err != nil {
		return schedule.Schedule{}, err
	}
	state.NextRunAt, err = decodeScheduleTime(nextRunAt)
	if err != nil {
		return schedule.Schedule{}, err
	}
	state.CreatedAt, err = decodeTime(createdAt)
	if err != nil {
		return schedule.Schedule{}, err
	}
	state.UpdatedAt, err = decodeTime(updatedAt)
	if err != nil {
		return schedule.Schedule{}, err
	}
	return schedule.Rehydrate(state)
}

const occurrenceSelect = `
	SELECT id, schedule_id, title, instructions, workspace_path, provider, model, cron,
		schedule_enabled, schedule_last_run_at, schedule_next_run_at, schedule_revision,
		schedule_created_at, schedule_updated_at, due_at, fired_at, next_run_at,
		session_id, run_id, status, accepted_at
	FROM schedule_occurrences `

func insertScheduleOccurrence(ctx context.Context, transaction *sql.Tx, value schedule.Occurrence) error {
	state := value.State()
	snapshot := state.Schedule
	_, err := transaction.ExecContext(ctx, `
		INSERT INTO schedule_occurrences (
			id, schedule_id, title, instructions, workspace_path, provider, model, cron,
			schedule_enabled, schedule_last_run_at, schedule_next_run_at, schedule_revision,
			schedule_created_at, schedule_updated_at, due_at, fired_at, next_run_at,
			session_id, run_id, status, accepted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		state.ID, snapshot.ID, snapshot.Title, snapshot.Instructions, snapshot.Workspace,
		snapshot.Provider, snapshot.Model, snapshot.Cron, snapshot.Enabled,
		encodeScheduleTime(snapshot.LastRunAt), encodeScheduleTime(snapshot.NextRunAt), snapshot.Revision,
		encodeTime(snapshot.CreatedAt), encodeTime(snapshot.UpdatedAt), encodeTime(state.DueAt),
		encodeTime(state.FiredAt), encodeTime(state.NextRunAt), state.SessionID, state.RunID,
		state.Status, encodeScheduleTime(state.AcceptedAt),
	)
	if err != nil {
		return fmt.Errorf("sqlite: insert schedule occurrence %q: %w", state.ID, err)
	}
	return nil
}

func scanScheduleOccurrence(scan func(...any) error) (schedule.Occurrence, error) {
	var state schedule.OccurrenceState
	var enabled int
	var scheduleLastRunAt string
	var scheduleNextRunAt string
	var scheduleCreatedAt string
	var scheduleUpdatedAt string
	var dueAt string
	var firedAt string
	var nextRunAt string
	var acceptedAt string
	if err := scan(
		&state.ID, &state.Schedule.ID, &state.Schedule.Title, &state.Schedule.Instructions,
		&state.Schedule.Workspace, &state.Schedule.Provider, &state.Schedule.Model, &state.Schedule.Cron,
		&enabled, &scheduleLastRunAt, &scheduleNextRunAt, &state.Schedule.Revision,
		&scheduleCreatedAt, &scheduleUpdatedAt, &dueAt, &firedAt, &nextRunAt,
		&state.SessionID, &state.RunID, &state.Status, &acceptedAt,
	); err != nil {
		return schedule.Occurrence{}, err
	}
	state.Schedule.Enabled = enabled == 1
	var err error
	state.Schedule.LastRunAt, err = decodeScheduleTime(scheduleLastRunAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	state.Schedule.NextRunAt, err = decodeScheduleTime(scheduleNextRunAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	state.Schedule.CreatedAt, err = decodeTime(scheduleCreatedAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	state.Schedule.UpdatedAt, err = decodeTime(scheduleUpdatedAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	state.DueAt, err = decodeTime(dueAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	state.FiredAt, err = decodeTime(firedAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	state.NextRunAt, err = decodeTime(nextRunAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	state.AcceptedAt, err = decodeScheduleTime(acceptedAt)
	if err != nil {
		return schedule.Occurrence{}, err
	}
	return schedule.RehydrateOccurrence(state)
}

func encodeScheduleTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return encodeTime(value)
}

func decodeScheduleTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return decodeTime(value)
}
