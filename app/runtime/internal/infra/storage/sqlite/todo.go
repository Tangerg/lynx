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
	state, err := s.State(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	return state.Items, nil
}

// State returns the session's whole task-list projection. A session with no list
// yet is the zero state — no items, revision 0, no update time — and not an error:
// "nothing has been written" is a legitimate answer, and the only one that lets a
// client render an empty panel instead of a failure.
func (s *TodoStore) State(ctx context.Context, sessionID string) (todo.State, error) {
	var (
		itemsJSON string
		revision  uint64
		updatedNs int64
	)
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT items, revision, updated_at FROM todos WHERE session_id = ?`, sessionID).Scan(&itemsJSON, &revision, &updatedNs)
	if errors.Is(err, sql.ErrNoRows) {
		return todo.State{}, nil
	}
	if err != nil {
		return todo.State{}, fmt.Errorf("sqlite: read todos: %w", err)
	}
	state := todo.State{Revision: revision, UpdatedAt: time.Unix(0, updatedNs).UTC()}
	items, err := decodeTodoItems(itemsJSON)
	if err != nil {
		return todo.State{}, err
	}
	state.Items = items
	return state, nil
}

func decodeTodoItems(itemsJSON string) ([]todo.Item, error) {
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
	if err := todo.ValidateSnapshot(items); err != nil {
		return nil, fmt.Errorf("sqlite: validate todos: %w", err)
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
	if err := todo.ValidateSnapshot(items); err != nil {
		return fmt.Errorf("sqlite: validate todos: %w", err)
	}
	rows := make([]todoItemRow, len(items))
	for index, item := range items {
		rows[index] = todoItemRow{Content: item.Content, Status: item.Status, BlockedReason: item.BlockedReason, NextAction: item.NextAction}
	}
	data, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("sqlite: encode todos: %w", err)
	}
	// The revision is bumped by the write itself — SQL reads the current value and
	// adds one in the same statement — so two concurrent replacements cannot be
	// assigned the same number, and no caller has to remember to increment it.
	_, err = conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO todos(session_id, items, revision, updated_at) VALUES (?, ?, 1, ?)
		 ON CONFLICT(session_id) DO UPDATE SET
		   items = excluded.items,
		   revision = todos.revision + 1,
		   updated_at = excluded.updated_at`,
		sessionID, string(data), time.Now().UTC().UnixNano())
	if err != nil {
		return fmt.Errorf("sqlite: replace todos: %w", err)
	}
	return nil
}

// CaptureBoundary records the session's current list as the value runID's
// boundary holds — the moment that Run ended. It is a single statement so the
// recorded value is the one the live row holds inside the caller's transaction,
// and a session with no list yet records an empty one: "the list was empty when
// this Run ended" is a fact a rollback has to be able to restore, and it is not
// the same answer as "never captured".
//
// It INSERTs without an upsert on purpose: a Run reaches terminal exactly once
// (the transition is a CAS on its non-terminal state), so a second boundary for
// one Run would be a lifecycle bug and is left loud.
func (s *TodoStore) CaptureBoundary(ctx context.Context, sessionID, runID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`INSERT INTO todo_boundaries(run_id, items)
		 VALUES (?, COALESCE((SELECT items FROM todos WHERE session_id = ?), '[]'))`,
		runID, sessionID); err != nil {
		return fmt.Errorf("sqlite: capture todo boundary for run %q: %w", runID, err)
	}
	return nil
}

// Boundary returns the list runID's boundary recorded. false means no boundary
// was ever captured for that Run — an imported Run, whose portable Artifact
// carries the live value and no history — which is NOT an empty list: the caller
// must leave the live list alone rather than restore emptiness it cannot know.
func (s *TodoStore) Boundary(ctx context.Context, runID string) ([]todo.Item, bool, error) {
	var itemsJSON string
	err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT items FROM todo_boundaries WHERE run_id = ?`, runID).Scan(&itemsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("sqlite: read todo boundary for run %q: %w", runID, err)
	}
	items, err := decodeTodoItems(itemsJSON)
	if err != nil {
		return nil, false, err
	}
	return items, true, nil
}

// DeleteSession removes the todo projection owned by sessionID. It joins an
// ambient lifecycle write-set transaction through conn(ctx). Recorded boundaries
// belong to the Run rows and go with them.
func (s *TodoStore) DeleteSession(ctx context.Context, sessionID string) error {
	if _, err := conn(ctx, s.db).ExecContext(ctx, `DELETE FROM todos WHERE session_id = ?`, sessionID); err != nil {
		return fmt.Errorf("sqlite: delete session todos: %w", err)
	}
	return nil
}
