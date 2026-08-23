package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/session"
)

func (database *Database) CreateSession(ctx context.Context, value session.Session) error {
	_, err := database.database.ExecContext(ctx, `
		INSERT INTO sessions (
			id, title, workspace_path, model, favorite, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID().String(), value.Title(), value.Workspace().Path(), value.Model(),
		value.Favorite(), value.Revision(), encodeTime(value.CreatedAt()), encodeTime(value.UpdatedAt()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: create session %s: %w", value.ID(), err)
	}
	return nil
}

func (database *Database) GetSession(ctx context.Context, id session.ID) (session.Session, error) {
	return scanSession(database.database.QueryRowContext(ctx, `
		SELECT id, title, workspace_path, model, favorite, revision, created_at, updated_at
		FROM sessions WHERE id = ?`, id.String()))
}

func (database *Database) ListSessions(ctx context.Context, limit int, after *session.Cursor) (session.Page, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := `
		SELECT id, title, workspace_path, model, favorite, revision, created_at, updated_at
		FROM sessions`
	arguments := make([]any, 0, 3)
	if after != nil {
		query += ` WHERE (updated_at < ? OR (updated_at = ? AND id < ?))`
		arguments = append(arguments, encodeTime(after.UpdatedAt), encodeTime(after.UpdatedAt), after.ID)
	}
	query += ` ORDER BY favorite DESC, updated_at DESC, id DESC LIMIT ?`
	arguments = append(arguments, limit+1)
	rows, err := database.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return session.Page{}, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	defer rows.Close()
	values := make([]session.Session, 0, limit+1)
	for rows.Next() {
		value, err := scanSession(rows)
		if err != nil {
			return session.Page{}, err
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return session.Page{}, fmt.Errorf("sqlite: iterate sessions: %w", err)
	}
	page := session.Page{Sessions: values}
	if len(values) > limit {
		last := values[limit-1]
		page.Sessions = values[:limit]
		page.Next = &session.Cursor{UpdatedAt: last.UpdatedAt(), ID: last.ID().String()}
	}
	return page, nil
}

func (database *Database) UpdateSession(ctx context.Context, value session.Session, previousRevision uint64) error {
	result, err := database.database.ExecContext(ctx, `
		UPDATE sessions SET
			title = ?, workspace_path = ?, model = ?, favorite = ?, revision = ?, updated_at = ?
		WHERE id = ? AND revision = ?`,
		value.Title(), value.Workspace().Path(), value.Model(), value.Favorite(),
		value.Revision(), encodeTime(value.UpdatedAt()), value.ID().String(), previousRevision,
	)
	if err != nil {
		return fmt.Errorf("sqlite: update session %s: %w", value.ID(), err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect session update: %w", err)
	}
	if changed == 0 {
		if _, findErr := database.GetSession(ctx, value.ID()); errors.Is(findErr, session.ErrNotFound) {
			return session.ErrNotFound
		}
		return session.ErrRevisionConflict
	}
	return nil
}

func (database *Database) DeleteSession(ctx context.Context, id session.ID) error {
	result, err := database.database.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, id.String())
	if err != nil {
		return fmt.Errorf("sqlite: delete session %s: %w", id, err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("sqlite: inspect session delete: %w", err)
	}
	if changed == 0 {
		return session.ErrNotFound
	}
	return nil
}

type rowScanner interface{ Scan(...any) error }

func scanSession(row rowScanner) (session.Session, error) {
	var (
		id, title, workspacePath, model, created, updated string
		favorite bool
		revision uint64
	)
	if err := row.Scan(&id, &title, &workspacePath, &model, &favorite, &revision, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.Session{}, session.ErrNotFound
		}
		return session.Session{}, fmt.Errorf("sqlite: scan session: %w", err)
	}
	workspace, err := session.NewWorkspace(workspacePath)
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: restore session workspace: %w", err)
	}
	createdAt, err := decodeTime(created)
	if err != nil {
		return session.Session{}, err
	}
	updatedAt, err := decodeTime(updated)
	if err != nil {
		return session.Session{}, err
	}
	value, err := session.Rehydrate(session.Restore{
		ID: session.ID(id), Title: title, Workspace: workspace, Model: model,
		Favorite: favorite, Revision: revision, CreatedAt: createdAt, UpdatedAt: updatedAt,
	})
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: restore session %s: %w", id, err)
	}
	return value, nil
}

func encodeTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func decodeTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlite: decode timestamp: %w", err)
	}
	return parsed, nil
}
