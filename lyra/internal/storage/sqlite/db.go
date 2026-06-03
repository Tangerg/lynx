// Package sqlite hosts the SQLite-backed implementations of the
// service interfaces in lyra/internal/service/*. One SQLite file holds
// both sessions + memory under separate tables — callers share a
// single *sql.DB across the two services.
//
// Driver: modernc.org/sqlite (pure Go). No CGO, cross-compilation
// works out of the box.
package sqlite

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Open dials a SQLite database at path and applies the migrations
// every storage type in this package depends on. The returned *sql.DB
// is safe for concurrent use; callers share it across SessionService
// + MemoryService.
//
// Tuning baked in:
//   - journal_mode = WAL — concurrent readers don't block the writer
//   - foreign_keys = ON — surfaces parent-id violations early
//   - busy_timeout = 5000ms — survives brief contention from the
//     observer / trace writers piling onto the same connection
func Open(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)",
		path,
	)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	// modernc.org/sqlite serializes writes internally; one connection
	// is sufficient and avoids "database is locked" surprises under
	// concurrent transactions.
	db.SetMaxOpenConns(1)

	if err := migrate(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// migrate is a forward-only schema bootstrap. We deliberately don't
// version the migrations — there's only one schema today and the
// project is pre-1.0; when we ship a v2 schema we'll add a tracker
// table at the same time.
func migrate(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id          TEXT    PRIMARY KEY,
			title       TEXT    NOT NULL,
			parent_id   TEXT    NOT NULL DEFAULT '',
			started_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL,
			turn_count  INTEGER NOT NULL DEFAULT 0,
			metadata    TEXT    NOT NULL DEFAULT '{}'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at
			ON sessions(updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS memory (
			scope        INTEGER PRIMARY KEY,
			content      TEXT    NOT NULL,
			captured_at  TEXT    NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS process_snapshots (
			id           TEXT    PRIMARY KEY,
			snapshot     TEXT    NOT NULL,
			captured_at  INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS interrupts (
			parent_run_id TEXT    PRIMARY KEY,
			session_id    TEXT    NOT NULL DEFAULT '',
			turn_id       TEXT    NOT NULL DEFAULT '',
			process_id    TEXT    NOT NULL DEFAULT '',
			interrupts    TEXT    NOT NULL DEFAULT '',
			created_at    INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interrupts_session
			ON interrupts(session_id)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite: migrate: %w", err)
		}
	}
	return nil
}
