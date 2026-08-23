package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/conversation"
	rundomain "github.com/Tangerg/lynx/app2/runtime/domain/run"
	"github.com/Tangerg/lynx/app2/runtime/domain/transcript"
	"github.com/Tangerg/lynx/app2/runtime/runflow"
)

func (database *Database) CreateRun(
	ctx context.Context,
	record rundomain.Record,
	opening *transcript.Record,
	openingMessage *conversation.Record,
	events []rundomain.EventRecord,
) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin run creation: %w", err)
	}
	defer transaction.Rollback()
	if err := insertRun(ctx, transaction, record); err != nil {
		return err
	}
	if opening != nil {
		if err := insertItem(ctx, transaction, *opening); err != nil {
			return err
		}
	}
	if openingMessage != nil {
		if err := insertConversationMessage(ctx, transaction, *openingMessage); err != nil { return err }
	}
	if err := insertRunEvents(ctx, transaction, events); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit run creation: %w", err)
	}
	return nil
}

func (database *Database) CommitRun(ctx context.Context, write runflow.CommitWrite) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin run commit: %w", err)
	}
	defer transaction.Rollback()
	record := write.Run
	value := record.Run
	result, err := transaction.ExecContext(ctx, `
		UPDATE runs SET status = ?, active_segment_id = nullif(?, ''), outcome = nullif(?, ''), detail = ?, body = ?,
			updated_at = ?, finished_at = nullif(?, '')
		WHERE id = ? AND status = ? AND coalesce(active_segment_id, '') = ?`,
		string(value.Status()), value.ActiveSegmentID(), string(value.Outcome()), value.Detail(), string(record.Body),
		encodeTime(value.UpdatedAt()), encodeOptionalTime(value.FinishedAt()), value.ID(), string(write.ExpectedStatus), write.ExpectedSegmentID,
	)
	if err != nil {
		return fmt.Errorf("sqlite: commit run %s: %w", value.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect run commit: %w", err)
	}
	if changed == 0 {
		return rundomain.ErrInvalidTransition
	}
	for _, item := range write.Items {
		if err := putItem(ctx, transaction, item); err != nil {
			return err
		}
	}
	for _, message := range write.Messages {
		if err := insertConversationMessage(ctx, transaction, message); err != nil { return err }
	}
	for _, result := range write.ToolResults {
		if err := insertToolResult(ctx, transaction, result); err != nil { return err }
	}
	if err := insertRunEvents(ctx, transaction, write.Events); err != nil {
		return err
	}
	if value.Status() == rundomain.Finished {
		if _, err := transaction.ExecContext(ctx, `DELETE FROM interrupt_sets WHERE run_id = ?`, value.ID()); err != nil { return fmt.Errorf("sqlite: clear terminal interrupt: %w", err) }
		if _, err := transaction.ExecContext(ctx, `DELETE FROM executor_checkpoints WHERE run_id = ?`, value.ID()); err != nil { return fmt.Errorf("sqlite: clear terminal checkpoint: %w", err) }
		if value.ParentRunID() == "" {
			if err := capturePlanBoundary(ctx, transaction, value.ID(), value.SessionID()); err != nil { return err }
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: finish run commit: %w", err)
	}
	return nil
}

func capturePlanBoundary(ctx context.Context, transaction *sql.Tx, runID, sessionID string) error {
	var body string
	err := transaction.QueryRowContext(ctx, `SELECT body FROM plans WHERE session_id=?`, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		body, err = emptyPlanBody()
	}
	if err != nil { return fmt.Errorf("sqlite: read Plan boundary for run %s: %w", runID, err) }
	if _, err := transaction.ExecContext(ctx, `INSERT INTO plan_boundaries(run_id,body) VALUES(?,?)`, runID, body); err != nil {
		return fmt.Errorf("sqlite: capture Plan boundary for run %s: %w", runID, err)
	}
	return nil
}

func insertRunEvents(ctx context.Context, transaction *sql.Tx, events []rundomain.EventRecord) error {
	for _, event := range events {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO run_events (run_id, segment_id, event_id, ordinal, body, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, event.RunID, event.SegmentID, event.EventID,
			event.Ordinal, string(event.Body), encodeTime(event.CreatedAt)); err != nil {
			return fmt.Errorf("sqlite: append run event %s: %w", event.EventID, err)
		}
	}
	return nil
}

func (database *Database) ListRunEvents(
	ctx context.Context,
	runID, segmentID, afterEventID string,
) ([]rundomain.EventRecord, error) {
	afterOrdinal := 0
	if afterEventID != "" {
		err := database.database.QueryRowContext(ctx, `
			SELECT ordinal FROM run_events WHERE run_id = ? AND segment_id = ? AND event_id = ?`,
			runID, segmentID, afterEventID).Scan(&afterOrdinal)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: event cursor is unknown", rundomain.ErrStaleSegment)
		}
		if err != nil {
			return nil, fmt.Errorf("sqlite: resolve run event cursor: %w", err)
		}
	}
	rows, err := database.database.QueryContext(ctx, `
		SELECT run_id, segment_id, event_id, ordinal, body, created_at
		FROM run_events WHERE run_id = ? AND segment_id = ? AND ordinal > ?
		ORDER BY ordinal`, runID, segmentID, afterOrdinal)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list run events: %w", err)
	}
	defer rows.Close()
	records := make([]rundomain.EventRecord, 0)
	for rows.Next() {
		var record rundomain.EventRecord
		var created string
		if err := rows.Scan(&record.RunID, &record.SegmentID, &record.EventID, &record.Ordinal, &record.Body, &created); err != nil {
			return nil, fmt.Errorf("sqlite: scan run event: %w", err)
		}
		record.CreatedAt, err = decodeTime(created)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func insertRun(ctx context.Context, transaction *sql.Tx, record rundomain.Record) error {
	value := record.Run
	_, err := transaction.ExecContext(ctx, `
		INSERT INTO runs (
			id, session_id, parent_run_id, root_run_id, spawned_by_item_id,
			status, active_segment_id, provider, model, outcome, detail, body, created_at, updated_at, finished_at
		) VALUES (?, ?, nullif(?, ''), nullif(?, ''), nullif(?, ''), ?, nullif(?, ''), ?, ?, nullif(?, ''), ?, ?, ?, ?, nullif(?, ''))`,
		value.ID(), value.SessionID(), value.ParentRunID(), value.RootRunID(), value.SpawnedByItemID(),
		string(value.Status()), value.ActiveSegmentID(), value.Provider(), value.Model(), string(value.Outcome()), value.Detail(), string(record.Body),
		encodeTime(value.CreatedAt()), encodeTime(value.UpdatedAt()), encodeOptionalTime(value.FinishedAt()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: create run %s: %w", value.ID(), err)
	}
	return nil
}

func (database *Database) GetRun(ctx context.Context, id string) (rundomain.Record, error) {
	return scanRun(database.database.QueryRowContext(ctx, `
		SELECT id, session_id, coalesce(parent_run_id, ''), coalesce(root_run_id, ''),
			coalesce(spawned_by_item_id, ''), status, coalesce(active_segment_id, ''),
			provider, model, coalesce(outcome, ''), detail, body, created_at, updated_at, coalesce(finished_at, '')
		FROM runs WHERE id = ?`, id))
}

func (database *Database) GetOpenRootRun(ctx context.Context, sessionID string) (rundomain.Record, error) {
	return scanRun(database.database.QueryRowContext(ctx, `
		SELECT id, session_id, coalesce(parent_run_id, ''), coalesce(root_run_id, ''),
			coalesce(spawned_by_item_id, ''), status, coalesce(active_segment_id, ''),
			provider, model, coalesce(outcome, ''), detail, body, created_at, updated_at, coalesce(finished_at, '')
		FROM runs WHERE session_id = ? AND parent_run_id IS NULL AND status != 'finished'`, sessionID))
}

func (database *Database) ListRunningRuns(ctx context.Context) ([]rundomain.Record, error) {
	rows, err := database.database.QueryContext(ctx, `
		SELECT id, session_id, coalesce(parent_run_id, ''), coalesce(root_run_id, ''),
			coalesce(spawned_by_item_id, ''), status, coalesce(active_segment_id, ''),
			provider, model, coalesce(outcome, ''), detail, body, created_at, updated_at, coalesce(finished_at, '')
		FROM runs WHERE status = 'running' ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list running runs: %w", err)
	}
	defer rows.Close()
	records := make([]rundomain.Record, 0)
	for rows.Next() {
		record, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: iterate running runs: %w", err)
	}
	return records, nil
}

func (database *Database) ListRuns(
	ctx context.Context,
	sessionID string,
	statuses []rundomain.Status,
	includeDescendants bool,
	limit int,
	after *rundomain.Cursor,
) (rundomain.Page, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := `
		SELECT id, session_id, coalesce(parent_run_id, ''), coalesce(root_run_id, ''),
			coalesce(spawned_by_item_id, ''), status, coalesce(active_segment_id, ''),
			provider, model, coalesce(outcome, ''), detail, body, created_at, updated_at, coalesce(finished_at, '')
		FROM runs WHERE 1 = 1`
	arguments := make([]any, 0, 12)
	if sessionID != "" {
		query += ` AND session_id = ?`
		arguments = append(arguments, sessionID)
	}
	if !includeDescendants {
		query += ` AND parent_run_id IS NULL`
	}
	if len(statuses) > 0 {
		query += ` AND status IN (`
		for index, status := range statuses {
			if index > 0 {
				query += `, `
			}
			query += `?`
			arguments = append(arguments, string(status))
		}
		query += `)`
	}
	if after != nil {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		arguments = append(arguments, encodeTime(after.CreatedAt), encodeTime(after.CreatedAt), after.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	arguments = append(arguments, limit+1)
	rows, err := database.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return rundomain.Page{}, fmt.Errorf("sqlite: list runs: %w", err)
	}
	defer rows.Close()
	records := make([]rundomain.Record, 0, limit+1)
	for rows.Next() {
		record, err := scanRun(rows)
		if err != nil {
			return rundomain.Page{}, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return rundomain.Page{}, fmt.Errorf("sqlite: iterate runs: %w", err)
	}
	page := rundomain.Page{Records: records}
	if len(records) > limit {
		last := records[limit-1].Run
		page.Records = records[:limit]
		page.Next = &rundomain.Cursor{CreatedAt: last.CreatedAt(), ID: last.ID()}
	}
	return page, nil
}

func (database *Database) UpdateRun(ctx context.Context, record rundomain.Record) error {
	value := record.Run
	result, err := database.database.ExecContext(ctx, `
		UPDATE runs SET status = ?, active_segment_id = nullif(?, ''), outcome = nullif(?, ''), detail = ?, body = ?,
			updated_at = ?, finished_at = nullif(?, '') WHERE id = ?`,
		string(value.Status()), value.ActiveSegmentID(), string(value.Outcome()), value.Detail(), string(record.Body),
		encodeTime(value.UpdatedAt()), encodeOptionalTime(value.FinishedAt()), value.ID(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: update run %s: %w", value.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect run update: %w", err)
	}
	if changed == 0 {
		return rundomain.ErrNotFound
	}
	return nil
}

func (database *Database) AppendItem(ctx context.Context, item transcript.Record) error {
	transaction, err := database.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin item append: %w", err)
	}
	defer transaction.Rollback()
	if err := insertItem(ctx, transaction, item); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit item append: %w", err)
	}
	return nil
}

func insertItem(ctx context.Context, transaction *sql.Tx, item transcript.Record) error {
	_, err := transaction.ExecContext(ctx, `
		INSERT INTO items (id, session_id, run_id, ordinal, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`, item.ID, item.SessionID, item.RunID, item.Ordinal,
		string(item.Body), encodeTime(item.CreatedAt))
	if err != nil {
		return fmt.Errorf("sqlite: append item %s: %w", item.ID, err)
	}
	return nil
}

func putItem(ctx context.Context, transaction *sql.Tx, item transcript.Record) error {
	result, err := transaction.ExecContext(ctx, `
		INSERT INTO items (id, session_id, run_id, ordinal, body, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET body=excluded.body
		WHERE items.session_id=excluded.session_id AND items.run_id=excluded.run_id AND items.ordinal=excluded.ordinal`,
		item.ID, item.SessionID, item.RunID, item.Ordinal, string(item.Body), encodeTime(item.CreatedAt))
	if err != nil { return fmt.Errorf("sqlite: put item %s: %w", item.ID, err) }
	changed, _ := result.RowsAffected()
	if changed != 1 { return fmt.Errorf("sqlite: item %s identity conflicts with its owner or ordinal", item.ID) }
	return nil
}

func (database *Database) ListItems(ctx context.Context, sessionID, runID string) ([]transcript.Record, error) {
	query := `SELECT id, session_id, run_id, ordinal, body, created_at FROM items WHERE 1 = 1`
	var arguments []any
	if sessionID != "" {
		query += ` AND session_id = ?`
		arguments = append(arguments, sessionID)
	}
	if runID != "" {
		query += ` AND run_id = ?`
		arguments = append(arguments, runID)
	}
	query += ` ORDER BY created_at, run_id, ordinal`
	rows, err := database.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list items: %w", err)
	}
	defer rows.Close()
	items := make([]transcript.Record, 0)
	for rows.Next() {
		var item transcript.Record
		var created string
		if err := rows.Scan(&item.ID, &item.SessionID, &item.RunID, &item.Ordinal, &item.Body, &created); err != nil {
			return nil, fmt.Errorf("sqlite: scan item: %w", err)
		}
		item.CreatedAt, err = decodeTime(created)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanRun(row rowScanner) (rundomain.Record, error) {
	var (
		id, sessionID, parentID, rootID, spawnedBy, status, segmentID string
		providerID, model, outcome, detail, body, created, updated, finished string
	)
	if err := row.Scan(
		&id, &sessionID, &parentID, &rootID, &spawnedBy, &status, &segmentID,
		&providerID, &model, &outcome, &detail, &body, &created, &updated, &finished,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return rundomain.Record{}, rundomain.ErrNotFound
		}
		return rundomain.Record{}, fmt.Errorf("sqlite: scan run: %w", err)
	}
	createdAt, err := decodeTime(created)
	if err != nil {
		return rundomain.Record{}, err
	}
	updatedAt, err := decodeTime(updated)
	if err != nil {
		return rundomain.Record{}, err
	}
	finishedAt, err := decodeOptionalTime(finished)
	if err != nil {
		return rundomain.Record{}, err
	}
	value, err := rundomain.Rehydrate(rundomain.Restore{
		ID: id, SessionID: sessionID, ParentRunID: parentID, RootRunID: rootID,
		SpawnedByItemID: spawnedBy, Status: rundomain.Status(status), ActiveSegmentID: segmentID,
		Provider: providerID, Model: model, CreatedAt: createdAt, UpdatedAt: updatedAt,
		Outcome: rundomain.Outcome(outcome), Detail: detail, FinishedAt: finishedAt,
	})
	if err != nil {
		return rundomain.Record{}, fmt.Errorf("sqlite: restore run %s: %w", id, err)
	}
	return rundomain.Record{Run: value, Body: []byte(body)}, nil
}

func encodeOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return encodeTime(value)
}

func decodeOptionalTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return decodeTime(value)
}
