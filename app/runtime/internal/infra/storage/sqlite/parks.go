package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/lynx/agent/toolloop"
)

// parkStore persists [toolloop.ParkState] in SQLite — the tool_parks table
// created by [Open]'s migration, like every other table in this package.
type parkStore struct {
	db *sql.DB
}

var _ toolloop.ParkStore = (*parkStore)(nil)

// NewParkStore returns a [toolloop.ParkStore] backed by db. db must have
// been opened via [Open] so the migration ran.
func NewParkStore(db *sql.DB) toolloop.ParkStore {
	return &parkStore{db: db}
}

// Consume atomically reads AND deletes the parked round for a conversation
// (a single DELETE ... RETURNING), or returns (nil, nil) when nothing is
// parked — the [toolloop.ParkConsumer] contract. One statement means there is no
// read-succeeds-then-delete-fails window that could leave a stale round to
// hijack a later turn.
func (s *parkStore) Consume(ctx context.Context, conversationID string) (*toolloop.ParkState, error) {
	var assistant, done sql.NullString
	err := s.db.QueryRowContext(ctx,
		`DELETE FROM tool_parks WHERE conversation_id = ? RETURNING assistant, done`,
		conversationID,
	).Scan(&assistant, &done)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("sqlite: consume tool park: %w", err)
	}
	state := &toolloop.ParkState{}
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

func (s *parkStore) Write(ctx context.Context, conversationID string, state *toolloop.ParkState) error {
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
