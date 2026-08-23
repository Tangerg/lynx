package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

func (database *Database) ProjectHookTrusted(
	ctx context.Context,
	project string,
) (bool, error) {
	if !filepath.IsAbs(project) || filepath.Clean(project) != project {
		return false, errors.New("sqlite: hook project must be canonical and absolute")
	}
	var trusted int
	err := database.database.QueryRowContext(ctx, `
		SELECT 1 FROM trusted_hook_projects WHERE project_path = ?`,
		project,
	).Scan(&trusted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: read project hook trust: %w", err)
	}
	return trusted == 1, nil
}

func (database *Database) SetProjectHookTrusted(
	ctx context.Context,
	project string,
	trusted bool,
	now time.Time,
) (bool, error) {
	now = now.UTC()
	if !filepath.IsAbs(project) || filepath.Clean(project) != project || now.IsZero() {
		return false, errors.New("sqlite: invalid project hook trust mutation")
	}
	var (
		result sql.Result
		err    error
	)
	if trusted {
		result, err = database.database.ExecContext(ctx, `
			INSERT INTO trusted_hook_projects (project_path, trusted_at)
			VALUES (?, ?)
			ON CONFLICT(project_path) DO NOTHING`,
			project,
			encodeTime(now),
		)
	} else {
		result, err = database.database.ExecContext(ctx, `
			DELETE FROM trusted_hook_projects WHERE project_path = ?`,
			project,
		)
	}
	if err != nil {
		return false, fmt.Errorf("sqlite: set project hook trust: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("sqlite: inspect project hook trust: %w", err)
	}
	return changed > 0, nil
}
