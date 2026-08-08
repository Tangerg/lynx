// Package sqlite hosts the SQLite-backed implementations of Runtime's storage
// ports. One SQLite file is the single
// durable backend — sessions / executor checkpoints / interrupts / history /
// providers each live in their own table, sharing one *sql.DB. Human-authored
// memory is the deliberate exception: it stays a user-editable LYRA.md file
// cascade. Agent-extracted ledger and curated memory are ordinary SQLite state.
//
// Driver: modernc.org/sqlite (pure Go). No CGO, cross-compilation
// works out of the box.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// ErrSchemaEpochMismatch reports that the database file was written by a
// different schema epoch than this build installs. This pre-release runtime
// carries exactly one storage shape and no compatibility migrations, so such a
// file is refused rather than rewritten: dropping its tables would destroy the
// user's sessions, transcripts, and credentials to accommodate a developer's
// schema change, and that is the user's call, not the runtime's.
var ErrSchemaEpochMismatch = errors.New("sqlite: schema epoch mismatch")

// Open dials a SQLite database at path and installs the current schema. A file
// written by another schema epoch is refused with [ErrSchemaEpochMismatch]; a
// file that holds no schema yet is installed into. The returned *sql.DB is
// safe for concurrent use; callers share it across every
// sqlite-backed store (session / transcript / interrupt / provider / message /
// agent memory). Human-authored knowledge (LYRA.md) is file-backed, not here.
//
// Tuning baked in:
//   - journal_mode = WAL — concurrent readers don't block the writer
//   - foreign_keys = ON — surfaces parent-id violations early
//   - busy_timeout = 5000ms — survives brief contention from
//     concurrent writers piling onto the same connection
func Open(ctx context.Context, path string) (*sql.DB, error) {
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

	if err := installCurrentSchema(ctx, db, path); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return db, nil
}

// schemaEpoch identifies the one storage shape this build understands. It is an
// epoch rather than a version because nothing connects two values: a database
// stamped with any other number is refused, never upgraded.
const schemaEpoch = 62

func installCurrentSchema(ctx context.Context, db *sql.DB, path string) error {
	var epoch int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&epoch); err != nil {
		return fmt.Errorf("sqlite: read schema epoch: %w", err)
	}
	if epoch != schemaEpoch {
		empty, err := holdsNoSchema(ctx, db)
		if err != nil {
			return err
		}
		if !empty {
			// The WAL sidecars are named because the database is opened in WAL mode:
			// deleting only the main file leaves a -wal whose salt no longer matches
			// the one that replaces it.
			return fmt.Errorf(
				"%w: %s was written by epoch %d and this build installs %d; "+
					"pre-release builds do not migrate durable state, so delete that file "+
					"(along with its -wal and -shm sidecars) to start from an empty one",
				ErrSchemaEpochMismatch, path, epoch, schemaEpoch)
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
			model       TEXT    NOT NULL DEFAULT '',
			favorite    INTEGER NOT NULL DEFAULT 0,
			isolated    INTEGER NOT NULL DEFAULT 0,
			revision    INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_updated_at
			ON sessions(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_parent
			ON sessions(parent_id)`,
		`CREATE TABLE IF NOT EXISTS executor_checkpoints (
			root_member_id TEXT    PRIMARY KEY,
			session_id      TEXT    NOT NULL,
			build_id        TEXT    NOT NULL,
			payload         BLOB    NOT NULL,
			policy          TEXT    NOT NULL,
			usage           TEXT    NOT NULL,
			committed_at    INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_executor_checkpoints_session
			ON executor_checkpoints(session_id)`,
		// One row per root or child Run. state is the coarse admission position —
		// 'running' | 'waiting' | 'terminal' — and the partial unique index
		// below is the durable "one non-terminal root Run tree per Session"
		// guarantee. Descendants share that tree's admission and therefore do not
		// compete for a second Session slot.
		//
		// detail / problem explain WHY a Run stopped and are written by the
		// lifecycle transition that makes them true: a terminal state and the
		// failure explaining it land in one statement, so no row can claim one
		// without the other. steps / active_duration_ns / usage are a different
		// kind of fact — how much the Run has consumed — and are written by every
		// commit from the first segment on, because a running Run costs money and
		// a parked one has to report what it already spent. max_total_tokens / max_steps /
		// max_budget_usd are the allowance it was admitted under, frozen at
		// creation: a resume and a cross-restart rehydrate have to apply the same
		// caps the first segment did, and zero means uncapped.
		//
		// A Run's open interrupts are NOT here — the interrupts table owns them
		// and a read composes them, because two copies of one park would be two
		// answers to "what is this run waiting on".
		//
		// The capabilities column is the optional behavior admitted for the Run. The
		// admission INSERT is its ONLY writer: no later statement names the column,
		// which is how "frozen for the Run's whole life" is kept by construction
		// rather than by a check that could be forgotten.
		//
		// message_mark is the conversation message count captured when the Run
		// finished (post-compaction) — the per-run rollback/fork watermark
		// fork{fromRunId} truncate to. -1 is transcript.UnknownMessageMark: a Run
		// that has not finished has no watermark yet.
		`CREATE TABLE IF NOT EXISTS runs (
			run_id             TEXT    PRIMARY KEY,
			session_id         TEXT    NOT NULL,
			spawned_by_item_id TEXT    NOT NULL DEFAULT '',
			parent_run_id      TEXT    NOT NULL DEFAULT '',
			root_run_id        TEXT    NOT NULL DEFAULT '',
			state              TEXT    NOT NULL,
			active_segment_id  TEXT    NOT NULL DEFAULT '',
			outcome            TEXT    NOT NULL DEFAULT '',
			provider           TEXT    NOT NULL DEFAULT '',
			model              TEXT    NOT NULL DEFAULT '',
			goal_lease_id      TEXT    NOT NULL DEFAULT '',
			detail             TEXT    NOT NULL DEFAULT '',
			steps              INTEGER NOT NULL DEFAULT 0,
			active_duration_ns INTEGER NOT NULL DEFAULT 0,
			usage              TEXT    NOT NULL DEFAULT '',
			problem            TEXT    NOT NULL DEFAULT '',
			max_total_tokens         INTEGER NOT NULL DEFAULT 0,
			max_steps          INTEGER NOT NULL DEFAULT 0,
			max_budget_usd     REAL    NOT NULL DEFAULT 0,
			capabilities       TEXT    NOT NULL DEFAULT '',
			message_mark       INTEGER NOT NULL DEFAULT -1,
			started_at         INTEGER NOT NULL,
			finished_at        INTEGER NOT NULL DEFAULT 0,
			updated_at         INTEGER NOT NULL,
			CHECK (
				(spawned_by_item_id = '' AND parent_run_id = '' AND root_run_id = '') OR
				(spawned_by_item_id != '' AND parent_run_id != '' AND root_run_id != '' AND
				 parent_run_id != run_id AND root_run_id != run_id)
			),
			CHECK (root_run_id = '' OR goal_lease_id = '')
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_session_active
			ON runs(session_id) WHERE state != 'terminal' AND root_run_id = ''`,
		`CREATE INDEX IF NOT EXISTS idx_runs_session
			ON runs(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_root
			ON runs(root_run_id) WHERE root_run_id != ''`,
		`CREATE INDEX IF NOT EXISTS idx_runs_parent
			ON runs(parent_run_id) WHERE parent_run_id != ''`,
		// model_invocations is the provider-attempt journal. It deliberately stores
		// neither semantic response content nor accounting: those facts belong to
		// history_items and runs. A surviving started row proves only that the
		// external boundary was crossed without a durable terminal observation.
		`CREATE TABLE IF NOT EXISTS model_invocations (
			call_id     TEXT    PRIMARY KEY,
			session_id  TEXT    NOT NULL,
			run_id      TEXT    NOT NULL,
			segment_id  TEXT    NOT NULL,
			state       TEXT    NOT NULL,
			started_at  INTEGER NOT NULL,
			finished_at INTEGER NOT NULL DEFAULT 0,
			CHECK (state IN ('started', 'completed', 'failed', 'unknown')),
			CHECK (
				(state = 'started' AND finished_at = 0) OR
				(state != 'started' AND finished_at >= started_at)
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_model_invocations_run
			ON model_invocations(run_id, segment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_model_invocations_open
			ON model_invocations(state) WHERE state = 'started'`,
		// tool_invocations is the operational pre-call journal. Its independent row
		// lets concurrent calls start in scheduler order while history_items receives
		// only final semantic Items in the model's declared order.
		`CREATE TABLE IF NOT EXISTS tool_invocations (
			call_id     TEXT    NOT NULL,
			item_id     TEXT    NOT NULL,
			session_id  TEXT    NOT NULL,
			run_id      TEXT    NOT NULL,
			segment_id  TEXT    NOT NULL,
			state       TEXT    NOT NULL,
			started_at  INTEGER NOT NULL,
			finished_at INTEGER NOT NULL DEFAULT 0,
			CHECK (state IN ('started', 'completed', 'incomplete')),
			CHECK (
				(state = 'started' AND finished_at = 0) OR
				(state != 'started' AND finished_at >= started_at)
			),
			PRIMARY KEY (call_id, segment_id),
			UNIQUE (item_id, segment_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_invocations_run
			ON tool_invocations(run_id, segment_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_invocations_open
			ON tool_invocations(state) WHERE state = 'started'`,
		// One root-owned row per parked Run tree. payload is the client-facing
		// interrupt set; interrupt_bindings maps each item to the private executor
		// boundary it answers; continuations carries every waiting Run's
		// application/executor hand-off. All three are closed JSON values. Answer
		// claim changes the row to resuming; the next barrier replaces it and a
		// terminal/recovery write-set deletes it.
		`CREATE TABLE IF NOT EXISTS interrupts (
			root_run_id        TEXT    PRIMARY KEY,
			session_id         TEXT    NOT NULL,
			executor_id        TEXT    NOT NULL,
			goal_lease_id      TEXT    NOT NULL DEFAULT '',
			-- Derived from the root Continuation and checked again on decode. It
			-- exists as a relational key so two pending sets cannot claim the same
			-- executor snapshot even though the complete hand-off stays one JSON
			-- value for atomic consume.
			root_member_id    TEXT    NOT NULL,
			payload            TEXT    NOT NULL,
			continuations      TEXT    NOT NULL,
			interrupt_bindings TEXT   NOT NULL,
			capabilities       TEXT    NOT NULL DEFAULT '',
			created_at         INTEGER NOT NULL,
			state              TEXT    NOT NULL DEFAULT 'open',
			answers            TEXT    NOT NULL DEFAULT '',
			claimed_at         INTEGER NOT NULL DEFAULT 0,
			CHECK (state IN ('open', 'resuming')),
			CHECK (
				(state = 'open' AND answers = '' AND claimed_at = 0) OR
				(state = 'resuming' AND answers != '' AND claimed_at > 0)
			)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_interrupts_session
			ON interrupts(session_id, state)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_interrupts_root_member
			ON interrupts(root_member_id)`,
		// pending_workspace_mutations is the recoverable operation log for file
		// rollbacks (§8.5). Git reset is non-atomic across paths; files+history also
		// spans Git and SQLite. The intent is logged before the tree is touched and
		// cleared after every requested effect commits. A surviving row is re-driven
		// at boot. session_id keys it — the mutation slot admits at most one
		// in-flight rollback per session. created_at is operational metadata only.
		`CREATE TABLE IF NOT EXISTS pending_workspace_mutations (
			session_id     TEXT    PRIMARY KEY,
			cwd            TEXT    NOT NULL,
			to_run_id      TEXT    NOT NULL,
			restore_history INTEGER NOT NULL,
			created_at     INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
		`CREATE TABLE IF NOT EXISTS history_items (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id  TEXT    NOT NULL,
			run_id      TEXT    NOT NULL DEFAULT '',
			item_id     TEXT    NOT NULL UNIQUE,
			occurred_at INTEGER NOT NULL,
			payload     TEXT    NOT NULL,
			offload_id  TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_history_items_session
			ON history_items(session_id, seq)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_history_items_offload
			ON history_items(offload_id) WHERE offload_id != ''`,
		// Full-text index over past conversation transcripts:
		// the human-readable user + agent message text, write-through from
		// history_items and keyed by the same seq (the FTS rowid), so a search
		// spans every session's conversation. The other columns are stored
		// UNINDEXED for retrieval/provenance only. porter stemming over unicode61
		// favors recall ("did we discuss X"); CJK runs tokenize coarsely (no ICU
		// tokenizer in the pure-Go driver). This is the repo's first FTS5 table —
		// discardSchema drops its shadow tables via the virtual table (see below).
		`CREATE VIRTUAL TABLE IF NOT EXISTS transcript_search USING fts5(
			text,
			session_id UNINDEXED,
			run_id UNINDEXED,
			item_id UNINDEXED,
			kind UNINDEXED,
			created_at UNINDEXED,
			tokenize = 'porter unicode61 remove_diacritics 2'
		)`,
		`CREATE TABLE IF NOT EXISTS providers (
			id        TEXT PRIMARY KEY,
			api_key   TEXT NOT NULL DEFAULT '',
			base_url  TEXT NOT NULL DEFAULT ''
		)`,
		// Global utility-model role: the (provider, model)
		// the in-house maintenance services — compaction / extraction / titling —
		// run on. Single row, pinned by CHECK(id = 1); empty model = unset (those
		// run on the main Run model).
		`CREATE TABLE IF NOT EXISTS utility_role (
			id        INTEGER PRIMARY KEY CHECK (id = 1),
			provider  TEXT NOT NULL DEFAULT '',
			model     TEXT NOT NULL DEFAULT ''
		)`,
		// MCP-server registry (mcp.servers.create/update). One row per server
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
		// OAuth owns an opaque, versioned payload in the MCP connection layer;
		// SQLite only enforces lifecycle and origin binding. The FK cascade removes
		// credentials with their server. A transport or endpoint change invalidates
		// the old credential before that server can reconnect elsewhere.
		`CREATE TABLE IF NOT EXISTS mcp_oauth_sessions (
			server_name TEXT PRIMARY KEY REFERENCES mcp_servers(name) ON DELETE CASCADE,
			origin      TEXT NOT NULL,
			payload     BLOB NOT NULL
		)`,
		`CREATE TRIGGER IF NOT EXISTS invalidate_mcp_oauth_session
			AFTER UPDATE OF transport, url, authorization, headers ON mcp_servers
			WHEN OLD.transport <> NEW.transport OR OLD.url <> NEW.url OR
			     OLD.authorization <> NEW.authorization OR OLD.headers <> NEW.headers
			BEGIN
				DELETE FROM mcp_oauth_sessions WHERE server_name = NEW.name;
			END`,
		`CREATE TABLE IF NOT EXISTS messages (
			seq             INTEGER PRIMARY KEY AUTOINCREMENT,
			conversation_id TEXT    NOT NULL,
			message         TEXT    NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_messages_conversation
			ON messages(conversation_id, seq)`,
		// A Plan is one complete ordered value per session. revision is assigned by
		// the replacement, not a clock, so clients can reject a late older snapshot.
		`CREATE TABLE IF NOT EXISTS session_plans (
			session_id TEXT    PRIMARY KEY,
			steps      TEXT    NOT NULL,
			revision   INTEGER NOT NULL CHECK (revision > 0),
			updated_at INTEGER NOT NULL
		)`,
		// A session gets an explicit permission row only after entering Plan mode.
		// The row retains the exact mode to restore on exit and follows the owning
		// session through the database FK lifecycle.
		`CREATE TABLE IF NOT EXISTS session_permission_modes (
			session_id   TEXT    PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
			mode         INTEGER NOT NULL,
			restore_mode INTEGER NOT NULL
		)`,
		// One row per terminal Run: the session's Plan as it stood when that Run
		// ended. session_plans is a latest-value projection with no history, so without
		// this row "the Plan at Run X" is unknowable — rollback and fork both
		// restore or copy a Run boundary. It is the state half of what
		// runs.message_mark is for the conversation, written by the same terminal
		// transition.
		//
		// The FK cascade IS the lifecycle: a Run dropped by a rollback or replaced by
		// an import takes its boundary with it, so no write-set has to remember to.
		// A MISSING row is not an empty list — it is "this moment was never captured",
		// which is the honest state of an imported Run (the portable Artifact carries
		// the live value only). Readers leave the live list alone rather than guessing
		// at empty, the same way an unknown message watermark leaves the log alone.
		`CREATE TABLE IF NOT EXISTS plan_boundaries (
			run_id TEXT PRIMARY KEY REFERENCES runs(run_id) ON DELETE CASCADE,
			steps  TEXT NOT NULL
		)`,
		// One autonomous goal per session (Goal mode). The FK is the durable
		// ownership invariant: a Goal cannot survive or be created after its
		// Session. budget/used are small JSON blobs read/written whole with the row.
		`CREATE TABLE IF NOT EXISTS goals (
			session_id TEXT    PRIMARY KEY REFERENCES sessions(id) ON DELETE CASCADE,
			objective  TEXT    NOT NULL,
			status     TEXT    NOT NULL,
			reason_code   TEXT    NOT NULL DEFAULT '',
			reason_detail TEXT    NOT NULL DEFAULT '',
			provider   TEXT    NOT NULL DEFAULT '',
			model      TEXT    NOT NULL DEFAULT '',
			budget     TEXT    NOT NULL,
			used       TEXT    NOT NULL,
			lease_id   TEXT    NOT NULL CHECK (lease_id <> ''),
			revision   INTEGER NOT NULL CHECK (revision > 0),
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		// One immutable row per terminal goal-owned Run. This is not a cache of
		// Goal.Used: it is the idempotency identity that lets terminal Run state
		// and cross-Run budget accounting commit as one fact.
		`CREATE TABLE IF NOT EXISTS goal_runs (
				run_id       TEXT    PRIMARY KEY,
				session_id   TEXT    NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
				lease_id     TEXT    NOT NULL,
			outcome      TEXT    NOT NULL,
			cost_usd     REAL    NOT NULL,
			steps        INTEGER NOT NULL,
			completed_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_goal_runs_session
			ON goal_runs(session_id, lease_id)`,
		// Persistent fine-grained approval rules. id is
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
		// Scheduled runs (schedules.*): a saved instructions fired on a cron trigger as
		// a headless run. last_run_at / next_run_at are unix millis (0 = never /
		// unscheduled); next_run_at is the worker's due index.
		`CREATE TABLE IF NOT EXISTS schedules (
			id          TEXT    PRIMARY KEY,
			title       TEXT    NOT NULL DEFAULT '',
			instructions      TEXT    NOT NULL,
			cwd         TEXT    NOT NULL DEFAULT '',
			provider    TEXT    NOT NULL DEFAULT '',
			model       TEXT    NOT NULL DEFAULT '',
			cron        TEXT    NOT NULL,
			enabled     INTEGER NOT NULL DEFAULT 1,
			last_run_at INTEGER NOT NULL DEFAULT 0,
			next_run_at INTEGER NOT NULL DEFAULT 0,
			created_at  INTEGER NOT NULL,
			revision    INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedules_due
			ON schedules(enabled, next_run_at)`,
		`CREATE TABLE IF NOT EXISTS schedule_firings (
			id          TEXT    PRIMARY KEY,
			schedule_id TEXT    NOT NULL,
			title       TEXT    NOT NULL DEFAULT '',
			instructions      TEXT    NOT NULL,
			cwd         TEXT    NOT NULL DEFAULT '',
			provider    TEXT    NOT NULL DEFAULT '',
			model       TEXT    NOT NULL DEFAULT '',
			cron        TEXT    NOT NULL,
			due_at      INTEGER NOT NULL,
			fired_at    INTEGER NOT NULL,
			next_run_at INTEGER NOT NULL,
			session_id  TEXT    NOT NULL UNIQUE,
			run_id      TEXT    NOT NULL UNIQUE,
			state       TEXT    NOT NULL CHECK(state IN ('pending', 'accepted'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schedule_firings_pending
			ON schedule_firings(state, due_at, id)`,
		// A schedule has one recoverable occurrence at a time. Keeping this as a
		// partial uniqueness invariant prevents each later cron tick from adding
		// another pending firing while Run admission is temporarily unavailable.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_schedule_firings_schedule_pending
			ON schedule_firings(schedule_id) WHERE state = 'pending'`,
		`CREATE TABLE IF NOT EXISTS idempotency_records (
			key         TEXT PRIMARY KEY,
			fingerprint TEXT NOT NULL,
			payload     BLOB NOT NULL,
			created_at  INTEGER NOT NULL,
			expires_at  INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_idempotency_records_expires_at
			ON idempotency_records(expires_at)`,
		// Embedding-model role: the (provider, model)
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
		// Offloaded tool-result bodies (context eviction): a single tool output
		// that exceeds the eviction threshold is moved here and model history keeps
		// only a bounded head+tail preview. history_items.offload_id + item_id form
		// the typed one-to-one relationship used to hydrate transcript reads; the
		// the compact preview carries id only for deferred result reads. session_id
		// scopes read-back, export, and delete; created_at orders portable records.
		`CREATE TABLE IF NOT EXISTS tool_result_blobs (
			id          TEXT    PRIMARY KEY,
			session_id  TEXT    NOT NULL DEFAULT '',
			item_id     TEXT    NOT NULL DEFAULT '',
			tool_name   TEXT    NOT NULL DEFAULT '',
			preview     TEXT    NOT NULL DEFAULT '',
			body        TEXT    NOT NULL,
			created_at  INTEGER NOT NULL DEFAULT (strftime('%s','now'))
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_result_blobs_session
			ON tool_result_blobs(session_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_result_blobs_item
			ON tool_result_blobs(item_id) WHERE item_id != ''`,
		// Append-only quality signals submitted through feedback.create. They are
		// deliberately not foreign-keyed: a user may report a general issue or
		// submit feedback after the referenced runtime records are cleaned up.
		`CREATE TABLE IF NOT EXISTS feedback_entries (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT    NOT NULL DEFAULT '',
			run_id     TEXT    NOT NULL DEFAULT '',
			item_id    TEXT    NOT NULL DEFAULT '',
			rating     TEXT    NOT NULL DEFAULT '',
			text       TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_feedback_entries_created
			ON feedback_entries(created_at DESC)`,
		// Append-only per-project fact ledger. day is the daily-ledger partition
		// (YYYY-MM-DD); seq is both stable ordering and the curation watermark.
		// A content digest deduplicates facts independently, so mixed old/new
		// extraction batches never lose their new members.
		`CREATE TABLE IF NOT EXISTS agent_memory_ledger (
			seq         INTEGER PRIMARY KEY AUTOINCREMENT,
			project     TEXT    NOT NULL,
			day         TEXT    NOT NULL,
			session_id  TEXT    NOT NULL,
			fact        TEXT    NOT NULL,
			digest      TEXT    NOT NULL,
			captured_at INTEGER NOT NULL,
			UNIQUE(project, digest)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_memory_ledger_project
			ON agent_memory_ledger(project, seq)`,
		// Curated memory items: the addressable projection folded from the ledger.
		// digest is the content identity (a reconcile matches unchanged items by
		// it to keep their id stable); the unique (scope, project, digest) index
		// dedups a fact across auto/user/pinned rows. origin 'auto' | 'user',
		// scope 'project' | 'user'. Pinned items are always injected and never
		// auto-pruned. session_id/day carry provenance.
		`CREATE TABLE IF NOT EXISTS agent_memory_items (
			id         TEXT    PRIMARY KEY,
			scope      TEXT    NOT NULL CHECK (scope IN ('project', 'user')),
			project    TEXT    NOT NULL DEFAULT '',
			content    TEXT    NOT NULL,
			digest     TEXT    NOT NULL,
			origin     TEXT    NOT NULL CHECK (origin IN ('auto', 'user')),
			-- HITL review lifecycle: 'active' (approved/injected/searched),
			-- 'pending' (proposed, awaiting review), 'rejected' (tombstone that
			-- blocks the same fact from being re-proposed).
			status     TEXT    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'pending', 'rejected')),
			pinned     INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
			session_id TEXT    NOT NULL DEFAULT '',
			day        TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			-- Content vector for semantic search (little-endian float32 BLOB, as in
			-- codebase_chunks). Empty until a configured embedder backfills it; a
			-- keyword scan works without it.
			embedding  BLOB    NOT NULL DEFAULT x'',
			CHECK ((scope = 'project' AND project <> '') OR (scope = 'user' AND project = '')),
			CHECK (origin <> 'user' OR status = 'active'),
			UNIQUE(scope, project, digest)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_agent_memory_items_scope
			ON agent_memory_items(scope, project)`,
		// Per-project curation watermark (the highest ledger seq already folded
		// into items). Kept apart from the items so a reconcile advances it with a
		// single compare-and-swap update.
		`CREATE TABLE IF NOT EXISTS agent_memory_state (
			project    TEXT    PRIMARY KEY,
			watermark  INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("sqlite: install current schema: %w", err)
		}
	}
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, schemaEpoch)); err != nil {
		return fmt.Errorf("sqlite: set schema epoch: %w", err)
	}
	return nil
}

// holdsNoSchema reports whether the database carries no tables of its own — the
// file this process just created. It is the one case where an epoch mismatch is
// not a mismatch at all: an unstamped empty file has no durable state to lose,
// so the current schema is installed into it.
func holdsNoSchema(ctx context.Context, db *sql.DB) (bool, error) {
	var tables int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	).Scan(&tables); err != nil {
		return false, fmt.Errorf("sqlite: inspect schema: %w", err)
	}
	return tables == 0, nil
}
