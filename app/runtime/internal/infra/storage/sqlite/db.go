// Package sqlite hosts the SQLite-backed implementations of Runtime's storage
// ports. One SQLite file is the single
// durable backend — sessions / process snapshots / interrupts / history /
// providers each live in their own table, sharing one *sql.DB. (Memory is
// the deliberate exception: it stays a user-editable LYRA.md file cascade,
// so it isn't stored here.)
//
// Driver: modernc.org/sqlite (pure Go). No CGO, cross-compilation
// works out of the box.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Open dials a SQLite database at path and installs the current schema. A
// mismatched development schema is discarded instead of migrated. The returned *sql.DB
// is safe for concurrent use; callers share it across every
// sqlite-backed store (session / transcript / interrupt / provider /
// message). Knowledge (LYRA.md) is file-backed, not here.
//
// Tuning baked in:
//   - journal_mode = WAL — concurrent readers don't block the writer
//   - foreign_keys = ON — surfaces parent-id violations early
//   - busy_timeout = 5000ms — survives brief contention from
//     concurrent writers piling onto the same connection
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

	if err := installCurrentSchema(db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

const schemaVersion = 3

func installCurrentSchema(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("sqlite: read schema version: %w", err)
	}
	if version != schemaVersion {
		if err := discardSchema(db); err != nil {
			return err
		}
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id          TEXT    PRIMARY KEY,
			title       TEXT    NOT NULL,
			cwd         TEXT    NOT NULL DEFAULT '',
			parent_id   TEXT    NOT NULL DEFAULT '',
			started_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL,
			metadata    TEXT    NOT NULL DEFAULT '{}',
			model       TEXT    NOT NULL DEFAULT '',
			kind        TEXT    NOT NULL DEFAULT '',
			favorite    INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at
			ON sessions(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_parent
			ON sessions(parent_id)`,
		`CREATE TABLE IF NOT EXISTS process_snapshots (
			id           TEXT    PRIMARY KEY,
			snapshot     TEXT    NOT NULL,
			captured_at  INTEGER NOT NULL
		)`,
		// Authoritative Run admission state (§8.2): one row per Run. state is the
		// coarse admission position — 'running' | 'interrupted' | 'terminal' — and
		// the partial unique index below is the durable "one non-terminal Run per
		// Session" guarantee that survives restart (the in-process live-run registry
		// is only this process's view). outcome is the terminal reason, '' until
		// terminal. Executor recovery handles live on open interrupts, where the
		// actual snapshot id is known.
		`CREATE TABLE IF NOT EXISTS runs (
			run_id      TEXT    PRIMARY KEY,
			session_id  TEXT    NOT NULL,
			state       TEXT    NOT NULL,
			provider    TEXT    NOT NULL DEFAULT '',
			model       TEXT    NOT NULL DEFAULT '',
			outcome     TEXT    NOT NULL DEFAULT '',
			started_at  INTEGER NOT NULL,
			updated_at  INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_session_active
			ON runs(session_id) WHERE state != 'terminal'`,
		`CREATE INDEX IF NOT EXISTS idx_runs_session
			ON runs(session_id)`,
		`CREATE TABLE IF NOT EXISTS interrupts (
			run_id         TEXT    PRIMARY KEY,
			session_id     TEXT    NOT NULL DEFAULT '',
			turn_id        TEXT    NOT NULL DEFAULT '',
			process_id     TEXT    NOT NULL DEFAULT '',
			provider       TEXT    NOT NULL DEFAULT '',
			model          TEXT    NOT NULL DEFAULT '',
			payload        TEXT    NOT NULL DEFAULT '',
			drained_tools  TEXT    NOT NULL DEFAULT '',
			run_created_at INTEGER NOT NULL DEFAULT 0,
			created_at     INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interrupts_session
			ON interrupts(session_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_interrupts_process
			ON interrupts(process_id) WHERE process_id != ''`,
		// pending_workspace_mutations is the recoverable operation log for file
		// rollbacks (§8.5): the working tree (git) and the durable history (SQLite)
		// can't commit in one ACID transaction, so the intent is logged before
		// either is touched and cleared once both commit. A row surviving a crash is
		// re-driven at boot. session_id keys it — the mutation slot admits at most
		// one in-flight rollback per session. created_at is stamped by the DB for
		// operational visibility (recovery is timestamp-free).
		`CREATE TABLE IF NOT EXISTS pending_workspace_mutations (
			session_id TEXT    PRIMARY KEY,
			cwd        TEXT    NOT NULL,
			to_run_id  TEXT    NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
		`CREATE TABLE IF NOT EXISTS history_items (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  TEXT    NOT NULL,
			run_id      TEXT    NOT NULL DEFAULT '',
			item_id     TEXT    NOT NULL UNIQUE,
			created_at  INTEGER NOT NULL,
			payload     TEXT    NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_history_items_session
			ON history_items(session_id, seq)`,
		`CREATE TABLE IF NOT EXISTS history_runs (
			run_id       TEXT    PRIMARY KEY,
			session_id   TEXT    NOT NULL,
			updated_at   INTEGER NOT NULL,
			payload      TEXT    NOT NULL,
			-- message_mark is the conversation message count captured when the run
			-- finished (post-compaction) — the per-run watermark sessions.rollback
			-- / fork{fromRunId} truncate to. -1 = unknown / still in-flight (B4).
			message_mark INTEGER NOT NULL DEFAULT -1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_history_runs_session
			ON history_runs(session_id)`,
		`CREATE TABLE IF NOT EXISTS providers (
			id        TEXT PRIMARY KEY,
			api_key   TEXT NOT NULL DEFAULT '',
			base_url  TEXT NOT NULL DEFAULT ''
		)`,
		// Global utility-model role (models.setUtilityRole): the (provider, model)
		// the in-house maintenance services — compaction / extraction / titling —
		// run on. Single row, pinned by CHECK(id = 1); empty model = unset (those
		// run on the main turn model).
		`CREATE TABLE IF NOT EXISTS utility_role (
			id        INTEGER PRIMARY KEY CHECK (id = 1),
			provider  TEXT NOT NULL DEFAULT '',
			model     TEXT NOT NULL DEFAULT ''
		)`,
		// MCP-server registry (workspace.mcp.configure). One row per server
		// name; the list columns (args/disabled_tools/auto_approve_tools) and the
		// map columns (env/headers) are JSON; timeout is nanoseconds. transport is
		// "stdio" | "streamableHttp".
		`CREATE TABLE IF NOT EXISTS mcp_servers (
			name               TEXT    PRIMARY KEY,
			transport          TEXT    NOT NULL,
			enabled            INTEGER NOT NULL DEFAULT 1,
			description        TEXT    NOT NULL DEFAULT '',
			url                TEXT    NOT NULL DEFAULT '',
			authorization      TEXT    NOT NULL DEFAULT '',
			headers            TEXT    NOT NULL DEFAULT '',
			command            TEXT    NOT NULL DEFAULT '',
			args               TEXT    NOT NULL DEFAULT '',
			env                TEXT    NOT NULL DEFAULT '',
			dir                TEXT    NOT NULL DEFAULT '',
			timeout            INTEGER NOT NULL DEFAULT 0,
			disabled_tools     TEXT    NOT NULL DEFAULT '',
			auto_approve_tools TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			seq             INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT    NOT NULL,
			message         TEXT    NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conversation
			ON messages(conversation_id, seq)`,
		`CREATE TABLE IF NOT EXISTS todos (
			session_id TEXT    PRIMARY KEY,
			items      TEXT    NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		// Persistent fine-grained approval rules (AUX_API §6). id is
		// deterministic over (scope, scope_key, tool, subject) so re-remembering
		// the same rule upserts the decision; scope_key is the session id /
		// project dir / '' for global.
		`CREATE TABLE IF NOT EXISTS approval_rules (
			id         TEXT    PRIMARY KEY,
			scope      TEXT    NOT NULL,
			scope_key  TEXT    NOT NULL DEFAULT '',
			tool       TEXT    NOT NULL,
			subject    TEXT    NOT NULL DEFAULT '',
			decision   TEXT    NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_approval_rules_scope
			ON approval_rules(scope, scope_key)`,
		// Projects whose .lyra/hooks.json is trusted to run. A cloned repo's
		// project-scope hooks must NOT auto-execute (supply-chain RCE); the user
		// trusts a project explicitly and the grant is recorded here. Global
		// (~/.lyra) hooks need no entry — they're the user's own.
		`CREATE TABLE IF NOT EXISTS trusted_projects (
			project_root TEXT    PRIMARY KEY,
			trusted_at   INTEGER NOT NULL
		)`,
		// Scheduled runs (schedules.*): a saved prompt fired on a cron trigger as
		// a headless run. last_run_at / next_run_at are unix millis (0 = never /
		// unscheduled); next_run_at is the worker's due index.
		`CREATE TABLE IF NOT EXISTS schedules (
			id          TEXT    PRIMARY KEY,
			title       TEXT    NOT NULL DEFAULT '',
			prompt      TEXT    NOT NULL,
			cwd         TEXT    NOT NULL DEFAULT '',
			provider    TEXT    NOT NULL DEFAULT '',
			model       TEXT    NOT NULL DEFAULT '',
			cron        TEXT    NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 1,
			last_run_at INTEGER NOT NULL DEFAULT 0,
			next_run_at INTEGER NOT NULL DEFAULT 0,
			created_at  INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedules_due
			ON schedules(enabled, next_run_at)`,
		// Embedding-model role (models.setEmbeddingRole): the (provider, model)
		// the @codebase semantic index embeds with. Single row, pinned by
		// CHECK(id = 1); empty model = unset (the index feature is off). Mirrors
		// utility_role; the credential comes from the provider registry.
		`CREATE TABLE IF NOT EXISTS embedding_role (
			id        INTEGER PRIMARY KEY CHECK (id = 1),
			provider  TEXT NOT NULL DEFAULT '',
			model     TEXT NOT NULL DEFAULT ''
		)`,
		// @codebase semantic index, keyed by project cwd. codebase_index is the
		// per-project meta (which embedding model the vectors were built with, so
		// a model change invalidates them; counts + timestamp for status).
		// codebase_files holds per-file content hashes for incremental re-index;
		// codebase_chunks holds the chunk text + its embedding (little-endian
		// float32 BLOB — half the size of float64, ample for cosine).
		`CREATE TABLE IF NOT EXISTS codebase_index (
			cwd         TEXT    PRIMARY KEY,
			model_id    TEXT    NOT NULL DEFAULT '',
			indexed_at  INTEGER NOT NULL DEFAULT 0,
			file_count  INTEGER NOT NULL DEFAULT 0,
			chunk_count INTEGER NOT NULL DEFAULT 0,
			truncated   INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS codebase_files (
			cwd  TEXT NOT NULL,
			path TEXT NOT NULL,
			hash TEXT NOT NULL,
			PRIMARY KEY (cwd, path)
		)`,
		`CREATE TABLE IF NOT EXISTS codebase_chunks (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			cwd        TEXT    NOT NULL,
			path       TEXT    NOT NULL,
			start_line INTEGER NOT NULL,
			end_line   INTEGER NOT NULL,
			text       TEXT    NOT NULL,
			embedding  BLOB    NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_codebase_chunks_cwd
			ON codebase_chunks(cwd)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("sqlite: install current schema: %w", err)
		}
	}
	if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, schemaVersion)); err != nil {
		return fmt.Errorf("sqlite: set schema version: %w", err)
	}
	return nil
}

func discardSchema(db *sql.DB) error {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return fmt.Errorf("sqlite: list stale schema: %w", err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			rows.Close()
			return fmt.Errorf("sqlite: scan stale schema: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("sqlite: close stale schema rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sqlite: list stale schema: %w", err)
	}
	for _, table := range tables {
		quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
		if _, err := db.Exec(`DROP TABLE ` + quoted); err != nil {
			return fmt.Errorf("sqlite: discard table %q: %w", table, err)
		}
	}
	return nil
}
