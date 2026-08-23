package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app2/runtime/domain/modelselection"
	"github.com/Tangerg/lynx/app2/runtime/domain/session"
)

func (database *Database) CreateSession(ctx context.Context, value session.Session) error {
	_, err := database.database.ExecContext(ctx, `
		INSERT INTO sessions (
			id, title, workspace_path, provider, model, favorite, revision, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		value.ID().String(), value.Title(), value.Workspace().Path(),
		value.Selection().Provider(), value.Selection().Model(),
		value.Favorite(), value.Revision(), encodeTime(value.CreatedAt()), encodeTime(value.UpdatedAt()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: create session %s: %w", value.ID(), err)
	}
	return nil
}

func (database *Database) GetSession(ctx context.Context, id session.ID) (session.Session, error) {
	return scanSession(database.database.QueryRowContext(ctx, `
		SELECT id, title, workspace_path, provider, model, favorite, revision, created_at, updated_at
		FROM sessions WHERE id = ?`, id.String()))
}

func (database *Database) GetSessionProjection(ctx context.Context, id session.ID) (session.Projection, error) {
	return scanSessionProjection(database.database.QueryRowContext(
		ctx,
		sessionProjectionSelect+` WHERE s.id = ?`,
		id.String(),
	))
}

func (database *Database) ListSessionProjections(ctx context.Context, limit int, after *session.Cursor) (session.Page, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := sessionProjectionSelect
	arguments := make([]any, 0, 6)
	if after != nil {
		query += ` WHERE (
			s.favorite < ?
			OR (s.favorite = ? AND (
				s.updated_at < ?
				OR (s.updated_at = ? AND s.id < ?)
			))
		)`
		arguments = append(
			arguments,
			after.Favorite,
			after.Favorite,
			encodeTime(after.UpdatedAt),
			encodeTime(after.UpdatedAt),
			after.ID,
		)
	}
	query += ` ORDER BY s.favorite DESC, s.updated_at DESC, s.id DESC LIMIT ?`
	arguments = append(arguments, limit+1)
	rows, err := database.database.QueryContext(ctx, query, arguments...)
	if err != nil {
		return session.Page{}, fmt.Errorf("sqlite: list sessions: %w", err)
	}
	defer rows.Close()
	projections := make([]session.Projection, 0, limit+1)
	for rows.Next() {
		projection, err := scanSessionProjection(rows)
		if err != nil {
			return session.Page{}, err
		}
		projections = append(projections, projection)
	}
	if err := rows.Err(); err != nil {
		return session.Page{}, fmt.Errorf("sqlite: iterate sessions: %w", err)
	}
	page := session.Page{Projections: projections}
	if len(projections) > limit {
		last := projections[limit-1].Session
		page.Projections = projections[:limit]
		page.Next = &session.Cursor{
			Favorite:  last.Favorite(),
			UpdatedAt: last.UpdatedAt(),
			ID:        last.ID().String(),
		}
	}
	return page, nil
}

const sessionProjectionSelect = `
	SELECT
		s.id, s.title, s.workspace_path, s.provider, s.model, s.favorite, s.revision,
		s.created_at, s.updated_at,
		CASE
			WHEN open_root.id IS NULL THEN ''
			WHEN EXISTS (
				SELECT 1
				FROM runs AS tree_run
				WHERE tree_run.session_id = s.id
					AND tree_run.status = 'running'
					AND (tree_run.id = open_root.id OR tree_run.root_run_id = open_root.id)
			) THEN 'running'
			ELSE 'waiting'
		END
	FROM sessions AS s
	LEFT JOIN runs AS open_root
		ON open_root.session_id = s.id
		AND open_root.parent_run_id IS NULL
		AND open_root.status != 'finished'`

func (database *Database) UpdateSession(ctx context.Context, value session.Session, previousRevision uint64) error {
	result, err := database.database.ExecContext(ctx, `
		UPDATE sessions SET
			title = ?, workspace_path = ?, provider = ?, model = ?, favorite = ?, revision = ?, updated_at = ?
		WHERE id = ? AND revision = ?`,
		value.Title(), value.Workspace().Path(), value.Selection().Provider(), value.Selection().Model(), value.Favorite(),
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
	var stored storedSession
	if err := scanSessionRow(row, stored.destinations()); err != nil {
		return session.Session{}, err
	}
	return stored.restore()
}

func scanSessionProjection(row rowScanner) (session.Projection, error) {
	var (
		stored storedSession
		status string
	)
	destinations := append(stored.destinations(), &status)
	if err := scanSessionRow(row, destinations); err != nil {
		return session.Projection{}, err
	}
	value, err := stored.restore()
	if err != nil {
		return session.Projection{}, err
	}
	if status == "" {
		status = string(session.StatusIdle)
	}
	projection, err := session.NewProjection(value, session.Status(status))
	if err != nil {
		return session.Projection{}, fmt.Errorf("sqlite: restore session projection %s: %w", value.ID(), err)
	}
	return projection, nil
}

type storedSession struct {
	id, title, workspacePath, provider, model, created, updated string
	favorite                                                    bool
	revision                                                    uint64
}

func (stored *storedSession) destinations() []any {
	return []any{
		&stored.id,
		&stored.title,
		&stored.workspacePath,
		&stored.provider,
		&stored.model,
		&stored.favorite,
		&stored.revision,
		&stored.created,
		&stored.updated,
	}
}

func (stored storedSession) restore() (session.Session, error) {
	workspace, err := session.NewWorkspace(stored.workspacePath)
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: restore session workspace: %w", err)
	}
	createdAt, err := decodeTime(stored.created)
	if err != nil {
		return session.Session{}, err
	}
	updatedAt, err := decodeTime(stored.updated)
	if err != nil {
		return session.Session{}, err
	}
	selection, err := modelselection.New(stored.provider, stored.model)
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: restore session model selection: %w", err)
	}
	value, err := session.Rehydrate(session.Restore{
		ID:        session.ID(stored.id),
		Title:     stored.title,
		Workspace: workspace,
		Selection: selection,
		Favorite:  stored.favorite,
		Revision:  stored.revision,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	})
	if err != nil {
		return session.Session{}, fmt.Errorf("sqlite: restore session %s: %w", stored.id, err)
	}
	return value, nil
}

func scanSessionRow(row rowScanner, destinations []any) error {
	if err := row.Scan(destinations...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return session.ErrNotFound
		}
		return fmt.Errorf("sqlite: scan session: %w", err)
	}
	return nil
}

func encodeTime(value time.Time) string { return value.UTC().Format(time.RFC3339Nano) }

func decodeTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlite: decode timestamp: %w", err)
	}
	return parsed, nil
}
