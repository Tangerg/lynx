package sqlite

import (
	"database/sql"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/session"
)

// SessionStore implements session.Store against a SQLite database.
// Mutations are single-row INSERT / UPDATE / DELETE, so each operation is
// atomic on its own — no multi-step rollback handling needed.
//
// All methods are safe for concurrent use; the underlying *sql.DB serializes
// writes when MaxOpenConns is 1 (see [Open]).
type SessionStore struct {
	db *sql.DB
}

var _ session.Store = (*SessionStore)(nil)

// NewSessionStore wires the given *sql.DB to the session.Store surface.
// The DB must have been opened via [Open] so the migration ran.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}
