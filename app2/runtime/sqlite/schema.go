package sqlite

import (
	"context"
	"database/sql"
	"fmt"
)

// createSchema installs the one app2 epoch. Tables follow state ownership; JSON
// bodies retain rich immutable values while indexed lifecycle facts remain
// relational and enforceable by SQLite.
func createSchema(ctx context.Context, database *sql.DB) error {
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin schema transaction: %w", err)
	}
	defer transaction.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			workspace_path TEXT NOT NULL,
			model TEXT NOT NULL,
			favorite INTEGER NOT NULL CHECK (favorite IN (0, 1)),
			revision INTEGER NOT NULL CHECK (revision > 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS sessions_catalog
			ON sessions(favorite DESC, updated_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS runs (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			parent_run_id TEXT REFERENCES runs(id) ON DELETE CASCADE,
			root_run_id TEXT,
			spawned_by_item_id TEXT,
			status TEXT NOT NULL CHECK (status IN ('running', 'waiting', 'finished')),
			active_segment_id TEXT,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			outcome TEXT,
			detail TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			finished_at TEXT,
			CHECK ((parent_run_id IS NULL AND root_run_id IS NULL AND spawned_by_item_id IS NULL)
				OR (parent_run_id IS NOT NULL AND root_run_id IS NOT NULL AND spawned_by_item_id IS NOT NULL)),
			CHECK ((status = 'running') = (active_segment_id IS NOT NULL)),
			CHECK ((status = 'finished') = (finished_at IS NOT NULL))
		) STRICT`,
		`CREATE UNIQUE INDEX IF NOT EXISTS one_open_root_tree_per_session
			ON runs(session_id) WHERE parent_run_id IS NULL AND status != 'finished'`,
		`CREATE INDEX IF NOT EXISTS runs_by_session
			ON runs(session_id, created_at, id)`,
		`CREATE TABLE IF NOT EXISTS items (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			UNIQUE(run_id, ordinal)
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS items_by_session
			ON items(session_id, created_at, id)`,
		`CREATE TABLE IF NOT EXISTS conversation_messages (
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
			body TEXT NOT NULL CHECK (json_valid(body)),
			PRIMARY KEY(session_id, ordinal)
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS conversation_messages_by_run
			ON conversation_messages(run_id, ordinal)`,
		`CREATE TABLE IF NOT EXISTS interrupt_sets (
			run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS executor_checkpoints (
			run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
			body BLOB NOT NULL,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS plans (
			session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
			revision INTEGER NOT NULL CHECK (revision >= 0),
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS goals (
			session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
			incarnation INTEGER NOT NULL CHECK (incarnation > 0),
			status TEXT NOT NULL,
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS run_events (
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			segment_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			ordinal INTEGER NOT NULL CHECK (ordinal > 0),
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			PRIMARY KEY(run_id, segment_id, event_id),
			UNIQUE(run_id, segment_id, ordinal)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS providers (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			secret BLOB,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS model_roles (
			role TEXT PRIMARY KEY CHECK (role IN ('utility', 'embedding')),
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS mcp_servers (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			secret BLOB,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS mcp_authorization_attempts (
			id TEXT PRIMARY KEY,
			server_id TEXT NOT NULL REFERENCES mcp_servers(id) ON DELETE CASCADE,
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS approval_state (
			singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS approval_rules (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS knowledge_entries (
			workspace_path TEXT NOT NULL,
			name TEXT NOT NULL,
			body TEXT NOT NULL CHECK (json_valid(body)),
			revision INTEGER NOT NULL CHECK (revision > 0),
			updated_at TEXT NOT NULL,
			PRIMARY KEY(workspace_path, name)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS agent_memory (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			status TEXT NOT NULL,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS feedback (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS hook_trust (
			workspace_path TEXT NOT NULL,
			hook_id TEXT NOT NULL,
			trusted INTEGER NOT NULL CHECK (trusted IN (0, 1)),
			updated_at TEXT NOT NULL,
			PRIMARY KEY(workspace_path, hook_id)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS managed_skills (
			name TEXT PRIMARY KEY,
			archived INTEGER NOT NULL CHECK (archived IN (0, 1)),
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS skill_proposals (
			workspace_path TEXT NOT NULL,
			name TEXT NOT NULL,
			revision TEXT NOT NULL,
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL,
			PRIMARY KEY(workspace_path, name)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS codebase_documents (
			workspace_path TEXT NOT NULL,
			path TEXT NOT NULL,
			body TEXT NOT NULL CHECK (json_valid(body)),
			indexed_at TEXT NOT NULL,
			PRIMARY KEY(workspace_path, path)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS codebase_indexes (
			workspace_path TEXT PRIMARY KEY,
			state TEXT NOT NULL CHECK (state IN ('none', 'indexing', 'ready', 'error')),
			operation_id TEXT,
			model_id TEXT,
			file_count INTEGER NOT NULL DEFAULT 0,
			chunk_count INTEGER NOT NULL DEFAULT 0,
			truncated INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
			indexed_at TEXT,
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS tool_results (
			id TEXT NOT NULL,
			item_id TEXT NOT NULL UNIQUE REFERENCES items(id) ON DELETE CASCADE,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			PRIMARY KEY(session_id, id)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS runtime_event_log (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			topic TEXT NOT NULL,
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS idempotency_outcomes (
			key TEXT PRIMARY KEY,
			fingerprint TEXT NOT NULL,
			state TEXT NOT NULL CHECK (state IN ('pending', 'complete')),
			body TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		) STRICT`,
	}
	for _, statement := range statements {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("sqlite: create app2 schema: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit app2 schema: %w", err)
	}
	return nil
}
