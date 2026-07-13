package sqlite

import "database/sql"

// SessionStore is the SQLite-backed session persistence surface — the single
// implementation each consumer's narrow session port (the sessions coordinator's
// lifecycle surface, the run-segment titler, the sub-agent spawn store) binds to.
// Mutations are single-row INSERT / UPDATE / DELETE, so each operation is
// atomic on its own — no multi-step rollback handling needed.
//
// All methods are safe for concurrent use; the underlying *sql.DB serializes
// writes when MaxOpenConns is 1 (see [Open]).
type SessionStore struct {
	db *sql.DB
}

// NewSessionStore wires a database opened via [Open] to the session persistence
// surface.
func NewSessionStore(db *sql.DB) *SessionStore {
	return &SessionStore{db: db}
}
