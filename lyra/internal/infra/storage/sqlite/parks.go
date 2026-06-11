package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/core/model/chat/middleware/tool"
)

// parkStore persists [tool.ParkState] in SQLite — the tool_parks table
// created by [Open]'s migration, like every other table in this package.
type parkStore struct {
	db *sql.DB
}

var _ tool.ParkStore = (*parkStore)(nil)

// NewParkStore returns a [tool.ParkStore] backed by db. db must have
// been opened via [Open] so the migration ran.
func NewParkStore(db *sql.DB) tool.ParkStore {
	return &parkStore{db: db}
}

// Read returns the parked round for a conversation, or (nil, nil) when
// nothing is parked — the [tool.ParkReader] contract.
func (s *parkStore) Read(ctx context.Context, conversationID string) (*tool.ParkState, error) {
	var assistant, done sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT assistant, done FROM tool_parks WHERE conversation_id = ?`,
		conversationID,
	).Scan(&assistant, &done)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: read tool park: %w", err)
	}
	state := &tool.ParkState{}
	if err := json.Unmarshal([]byte(assistant.String), &state.Assistant); err != nil {
		return nil, fmt.Errorf("sqlite: decode parked assistant: %w", err)
	}
	if done.Valid && done.String != "" {
		if err := json.Unmarshal([]byte(done.String), &state.Done); err != nil {
			return nil, fmt.Errorf("sqlite: decode parked tool results: %w", err)
		}
	}
	return state, nil
}

func (s *parkStore) Write(ctx context.Context, conversationID string, state *tool.ParkState) error {
	b, err := json.Marshal(state.Assistant)
	if err != nil {
		return fmt.Errorf("sqlite: encode parked assistant: %w", err)
	}
	var done []byte
	if len(state.Done) > 0 {
		done, err = json.Marshal(state.Done)
		if err != nil {
			return fmt.Errorf("sqlite: encode parked tool results: %w", err)
		}
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tool_parks(conversation_id, assistant, done, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(conversation_id) DO UPDATE SET
		   assistant = excluded.assistant,
		   done      = excluded.done`,
		conversationID, string(b), string(done), time.Now().UnixNano(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: write tool park: %w", err)
	}
	return nil
}

func (s *parkStore) Clear(ctx context.Context, conversationID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM tool_parks WHERE conversation_id = ?`, conversationID); err != nil {
		return fmt.Errorf("sqlite: clear tool park: %w", err)
	}
	return nil
}
