package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
)

// ToolResultStore persists offloaded tool-result bodies — the full output of a
// tool call that exceeded the context-eviction threshold. The oversized body is
// moved here and the conversation history keeps only a head+tail placeholder
// carrying the row id, so a single huge result stops bloating every subsequent
// LLM request while staying retrievable on demand (the read_tool_result tool
// fetches it back by id, paging with offset/limit).
//
// Rows are session-scoped so a deleted session's blobs drop in the same
// lifecycle cascade as its history (see [ToolResultStore.DropSession]); Fetch is
// likewise session-scoped so a session can only read back its own offloaded
// results. The DB must have been opened via [Open] so the tool_result_blobs
// table exists.
//
// Deliberately separate from the transcript (history_items also records tool
// results in full): this is the LLM's read-back store, keyed by an offload id
// and holding the raw body, whereas the transcript is the UI's record keyed by
// item id in a presenter shape. Serving read_tool_result from the transcript
// would need the wrong key, the wrong shape, and an execution→delivery
// dependency; storing the placeholder in the transcript instead would strip the
// UI's full-result view (regress it to a click-to-expand fetch). The bounded
// duplication of an oversized body (only bodies over the eviction threshold,
// session-scoped, dropped on delete) buys that decoupling and the full-inline UI
// — a deliberate trade-off, not an oversight. A rolled-back run's blobs are
// unreferenced until the session is deleted (self-healing on delete); keying
// blobs by run to drop them at rollback isn't worth the extra plumbing for that
// bounded, rare case.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type ToolResultStore struct {
	db *sql.DB
}

// NewToolResultStore wires a database with the current [Open]-installed schema
// to the offloaded-tool-result surface.
func NewToolResultStore(db *sql.DB) *ToolResultStore {
	return &ToolResultStore{db: db}
}

// Offload stores body under a freshly minted, unguessable id and returns that
// id for the placeholder that replaces the body in history. toolName is
// recorded for diagnostics only. It joins an ambient lifecycle write-set
// transaction through conn(ctx).
func (s *ToolResultStore) Offload(ctx context.Context, sessionID, toolName, body string) (string, error) {
	id := rand.Text()
	_, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO tool_result_blobs(id, session_id, tool_name, body, created_at)
		 VALUES (?, ?, ?, ?, strftime('%s','now'))`,
		id, sessionID, toolName, body)
	if err != nil {
		return "", fmt.Errorf("sqlite: offload tool result: %w", err)
	}
	return id, nil
}

// Fetch returns the full offloaded body for (sessionID, id). found is false —
// with a nil error — when no such row exists (an unknown id is a recoverable
// miss the caller surfaces to the model, not a failure). Scoping the read by
// session id keeps one session from reading another's offloaded output.
func (s *ToolResultStore) Fetch(ctx context.Context, sessionID, id string) (body string, found bool, err error) {
	err = conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT body FROM tool_result_blobs WHERE id = ? AND session_id = ?`,
		id, sessionID).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("sqlite: fetch tool result: %w", err)
	}
	return body, true, nil
}

// DropSession removes every offloaded body owned by sessionID — the blob half
// of the session-delete cascade. It joins an ambient lifecycle write-set
// transaction through conn(ctx).
func (s *ToolResultStore) DropSession(ctx context.Context, sessionID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM tool_result_blobs WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: drop session tool results: %w", err)
	}
	return nil
}
