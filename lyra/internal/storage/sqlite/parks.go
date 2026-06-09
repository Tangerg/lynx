package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
)

// parkStore persists [tool.ParkState] in SQLite.
type parkStore struct {
	db *sql.DB
}

// NewParkStore returns a [tool.ParkStore] backed by db.
// Creates the tool_parks table if it does not exist.
func NewParkStore(db *sql.DB) tool.ParkStore {
	_, _ = db.Exec(`CREATE TABLE IF NOT EXISTS tool_parks (
		conversation_id TEXT PRIMARY KEY,
		assistant      TEXT NOT NULL,
		done           TEXT,
		created_at     INTEGER NOT NULL DEFAULT (strftime('%s','now'))
	)`)
	return &parkStore{db: db}
}

func (s *parkStore) Read(ctx context.Context, conversationID string) (*tool.ParkState, error) {
	var assistant, done sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT assistant, done FROM tool_parks WHERE conversation_id = ?`,
		conversationID,
	).Scan(&assistant, &done)
	if err != nil {
		return nil, err
	}
	state := &tool.ParkState{}
	if err := json.Unmarshal([]byte(assistant.String), &state.Assistant); err != nil {
		return nil, err
	}
	if done.Valid && done.String != "" {
		if err := json.Unmarshal([]byte(done.String), &state.Done); err != nil {
			return nil, err
		}
	}
	return state, nil
}

func (s *parkStore) Write(ctx context.Context, conversationID string, state *tool.ParkState) error {
	b, err := json.Marshal(state.Assistant)
	if err != nil {
		return err
	}
	var done []byte
	if len(state.Done) > 0 {
		done, err = json.Marshal(state.Done)
		if err != nil {
			return err
		}
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO tool_parks (conversation_id, assistant, done) VALUES (?, ?, ?)`,
		conversationID, string(b), string(done),
	)
	return err
}

func (s *parkStore) Clear(ctx context.Context, conversationID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM tool_parks WHERE conversation_id = ?`, conversationID)
	return err
}

// Ensure parkStore satisfies tool.ParkStore.
var _ tool.ParkStore = (*parkStore)(nil)
