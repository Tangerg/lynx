package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/approval"
)

// PermissionModeStore persists the explicit permission state of sessions that
// entered Plan mode. Sessions without a row inherit the runtime default.
type PermissionModeStore struct{ db *sql.DB }

func NewPermissionModeStore(db *sql.DB) *PermissionModeStore {
	return &PermissionModeStore{db: db}
}

func (s *PermissionModeStore) GetMode(ctx context.Context, sessionID string) (approval.SessionMode, bool, error) {
	var state approval.SessionMode
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT mode, restore_mode FROM session_permission_modes WHERE session_id = ?`, sessionID,
	).Scan(&state.Mode, &state.RestoreMode)
	if errors.Is(err, sql.ErrNoRows) {
		return approval.SessionMode{}, false, nil
	}
	if err != nil {
		return approval.SessionMode{}, false, fmt.Errorf("sqlite: read session permission mode: %w", err)
	}
	if err := state.Validate(); err != nil {
		return approval.SessionMode{}, false, fmt.Errorf("sqlite: validate session permission mode: %w", err)
	}
	return state, true, nil
}

func (s *PermissionModeStore) PutMode(ctx context.Context, sessionID string, state approval.SessionMode) error {
	if sessionID == "" {
		return errors.New("sqlite: session permission mode requires a session id")
	}
	if err := state.Validate(); err != nil {
		return fmt.Errorf("sqlite: validate session permission mode: %w", err)
	}
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO session_permission_modes(session_id, mode, restore_mode) VALUES (?, ?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   mode = excluded.mode,
		   restore_mode = excluded.restore_mode`,
		sessionID, state.Mode, state.RestoreMode,
	); err != nil {
		return fmt.Errorf("sqlite: write session permission mode: %w", err)
	}
	return nil
}
