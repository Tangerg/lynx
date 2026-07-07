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

// TodoStore implements todo.Store against SQLite. A session's list is one
// row keyed by session id, the items a JSON array — the list is always read
// and written whole (a model-owned full replace), so a single row plus one
// UPSERT is the entire story; there are no per-item rows to reconcile.
//
// Safe for concurrent use; the *sql.DB serializes writes (MaxOpenConns 1, see
// [Open]).
type TodoStore struct {
	db *sql.DB
}

var _ todo.Store = (*TodoStore)(nil)

// NewTodoStore wires the *sql.DB (opened via [Open], so the migration ran)
// to the todo.Store surface.
func NewTodoStore(db *sql.DB) *TodoStore {
	return &TodoStore{db: db}
}

// List returns the session's items, or nil when the session has no list yet
// (an unknown session is not an error — see [todo.Store]).
func (s *TodoStore) List(ctx context.Context, sessionID string) ([]todo.Item, error) {
	var itemsJSON string
	err := s.db.QueryRowContext(ctx,
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
	var items []todo.Item
	if err := json.Unmarshal([]byte(itemsJSON), &items); err != nil {
		return nil, fmt.Errorf("sqlite: decode todos: %w", err)
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
	data, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("sqlite: encode todos: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO todos(session_id, items, updated_at) VALUES (?, ?, ?)`,
		sessionID, string(data), time.Now().UTC().UnixNano())
	if err != nil {
		return fmt.Errorf("sqlite: replace todos: %w", err)
	}
	return nil
}
