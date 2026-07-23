package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/app/runtime/internal/domain/todo"
)

// TodoStore is the SQLite persistence adapter for session todo lists. A session's list is one
// row keyed by session id, the items a JSON array — the list is always read
// and written whole (a model-owned full replace), so a single row plus one
// UPSERT is the entire story; there are no per-item rows to reconcile.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type TodoStore struct {
	db *sql.DB
}

type todoItemRow struct {
	Content       string      `json:"content"`
	Status        todo.Status `json:"status"`
	BlockedReason string      `json:"blocked_reason,omitempty"`
	NextAction    string      `json:"next_action,omitempty"`
}

// NewTodoStore wires a database with the current [Open]-installed schema to the
// todo persistence surface.
func NewTodoStore(db *sql.DB) *TodoStore {
	return &TodoStore{db: db}
}

// List returns the session's items, or nil when the session has no list yet
// (an unknown session is not an error).
func (s *TodoStore) List(ctx context.Context, sessionID string) ([]todo.Item, error) {
	var itemsJSON string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT items FROM todos WHERE session_id = ?`, sessionID).Scan(&itemsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: list todos: %w", err)
	}
	if itemsJSON == "" {
		return nil, nil
	}
	var rows []todoItemRow
	if err := json.Unmarshal([]byte(itemsJSON), &rows); err != nil {
		return nil, fmt.Errorf("sqlite: decode todos: %w", err)
	}
	items := make([]todo.Item, len(rows))
	for index, row := range rows {
		items[index] = todo.Item{Content: row.Content, Status: row.Status, BlockedReason: row.BlockedReason, NextAction: row.NextAction}
	}
	return items, nil
}

// Replace overwrites the session's list wholesale (INSERT OR REPLACE). A nil
// slice is stored as an empty array, so a cleared list round-trips as empty
// rather than NULL.
func (s *TodoStore) Replace(ctx context.Context, sessionID string, items []todo.Item) error {
	if items == nil {
		items = []todo.Item{}
	}
	rows := make([]todoItemRow, len(items))
	for index, item := range items {
		rows[index] = todoItemRow{Content: item.Content, Status: item.Status, BlockedReason: item.BlockedReason, NextAction: item.NextAction}
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("sqlite: encode todos: %w", err)
	}
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT OR REPLACE INTO todos(session_id, items, updated_at) VALUES (?, ?, ?)`,
		sessionID, string(data), time.Now().UTC().UnixNano())
	if err != nil {
		return fmt.Errorf("sqlite: replace todos: %w", err)
	}
	return nil
}

// DeleteSession removes the todo projection owned by sessionID. It joins an
// ambient lifecycle write-set transaction through conn(ctx).
func (s *TodoStore) DeleteSession(ctx context.Context, sessionID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM todos WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: delete session todos: %w", err)
	}
	return nil
}
