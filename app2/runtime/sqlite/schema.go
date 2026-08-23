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
			provider TEXT NOT NULL,
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
		`CREATE TABLE IF NOT EXISTS delegate_admissions (
			member_id TEXT PRIMARY KEY,
			parent_member_id TEXT NOT NULL,
			child_key TEXT NOT NULL,
			run_id TEXT NOT NULL UNIQUE,
			segment_id TEXT NOT NULL UNIQUE,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			parent_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			root_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			spawned_by_item_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			summary TEXT NOT NULL,
			instructions TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('pending', 'started', 'aborted')),
			failure TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(parent_member_id, child_key)
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS delegate_admissions_by_root
			ON delegate_admissions(root_run_id, started_at, member_id)`,
		`CREATE TABLE IF NOT EXISTS items (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			UNIQUE(run_id, ordinal)
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS items_timeline
			ON items(session_id, created_at, run_id, ordinal)`,
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
			revision INTEGER NOT NULL CHECK (revision > 0),
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS plan_modes (
			session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
			entered_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS plan_boundaries (
			run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
			body TEXT NOT NULL CHECK (json_valid(body))
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS goals (
			session_id TEXT PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
			incarnation_id TEXT NOT NULL,
			revision INTEGER NOT NULL CHECK (revision > 0),
			status TEXT NOT NULL CHECK (status IN ('active', 'paused', 'blocked', 'completing')),
			active_run_id TEXT REFERENCES runs(id) ON DELETE SET NULL,
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE UNIQUE INDEX IF NOT EXISTS one_goal_per_owned_run
			ON goals(active_run_id) WHERE active_run_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS run_events (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			root_run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			root_segment_id TEXT NOT NULL,
			run_id TEXT NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
			segment_id TEXT NOT NULL,
			event_id TEXT NOT NULL UNIQUE,
			ordinal INTEGER NOT NULL CHECK (ordinal > 0),
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL,
			UNIQUE(run_id, segment_id, ordinal)
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS run_events_by_tree_stream
			ON run_events(root_run_id, root_segment_id, sequence)`,
		`CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS providers (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			secret BLOB,
			revision INTEGER NOT NULL CHECK (revision > 0),
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
			revision INTEGER NOT NULL CHECK (revision > 0),
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
		`CREATE TABLE IF NOT EXISTS agent_memory (
			id TEXT PRIMARY KEY,
			scope TEXT NOT NULL CHECK (scope IN ('project', 'user')),
			project_path TEXT NOT NULL DEFAULT '',
			content TEXT NOT NULL CHECK (
				length(trim(content)) > 0 AND length(CAST(content AS BLOB)) <= 8192
			),
			digest TEXT NOT NULL CHECK (length(digest) = 64),
			origin TEXT NOT NULL CHECK (origin IN ('auto', 'user')),
			status TEXT NOT NULL CHECK (status IN ('active', 'pending', 'rejected')),
			pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
			session_id TEXT NOT NULL DEFAULT '',
			day TEXT NOT NULL CHECK (length(day) = 10),
			revision INTEGER NOT NULL CHECK (revision > 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			CHECK (
				(scope = 'project' AND length(project_path) > 0)
				OR (scope = 'user' AND project_path = '')
			),
			CHECK (origin != 'user' OR status = 'active'),
			CHECK (
				(origin = 'auto' AND length(session_id) > 0)
				OR (origin = 'user' AND session_id = '')
			),
			CHECK (pinned = 0 OR status = 'active'),
			UNIQUE(scope, project_path, digest)
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS agent_memory_target
			ON agent_memory(scope, project_path, status, pinned DESC, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS agent_memory_extractions (
			run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
			session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
			project_path TEXT NOT NULL CHECK (length(project_path) > 0),
			day TEXT NOT NULL CHECK (length(day) = 10),
			extractor_provider TEXT,
			extractor_model TEXT,
			completed_at TEXT NOT NULL,
			CHECK (
				(extractor_provider IS NULL AND extractor_model IS NULL)
				OR (
					extractor_provider IS NOT NULL AND extractor_model IS NOT NULL
					AND length(extractor_provider) > 0 AND length(extractor_model) > 0
				)
			)
		) STRICT`,
		`CREATE INDEX IF NOT EXISTS agent_memory_extractions_project
			ON agent_memory_extractions(project_path, run_id)`,
		`CREATE TABLE IF NOT EXISTS agent_memory_extraction_attempts (
			run_id TEXT PRIMARY KEY REFERENCES runs(id) ON DELETE CASCADE,
			attempted_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS agent_memory_ledger (
			sequence INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL REFERENCES agent_memory_extractions(run_id) ON DELETE CASCADE,
			content TEXT NOT NULL CHECK (
				length(trim(content)) > 0 AND length(CAST(content AS BLOB)) <= 2048
			),
			digest TEXT NOT NULL CHECK (length(digest) = 64),
			UNIQUE(run_id, digest)
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS agent_memory_curation (
			project_path TEXT PRIMARY KEY,
			watermark INTEGER NOT NULL CHECK (watermark >= 0),
			revision INTEGER NOT NULL CHECK (revision > 0),
			updated_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS feedback (
			id TEXT PRIMARY KEY,
			body TEXT NOT NULL CHECK (json_valid(body)),
			created_at TEXT NOT NULL
		) STRICT`,
		`CREATE TABLE IF NOT EXISTS trusted_hook_projects (
			project_path TEXT PRIMARY KEY,
			trusted_at TEXT NOT NULL
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
