package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/middleware/memory"
)

// MessageStore implements the lynx-core chat-memory [memory.Store] against
// SQLite — the per-conversation chat history the memory middleware loads
// before each turn and appends to after. One append-only table keyed by
// conversation, ordered by an autoincrement seq; each [chat.Message] is
// stored as opaque JSON (round-tripped via [chat.UnmarshalMessage]).
//
// Replaces the JSONL file store: history grows unboundedly per conversation,
// and an INSERT per message is the SQLite analog of the append-only file —
// O(1) writes, ordered reads, no whole-file rewrite.
type MessageStore struct {
	db *sql.DB
}

var _ memory.Store = (*MessageStore)(nil)

// NewMessageStore binds the chat-memory store to db. db must have been
// opened via [Open] so the migration ran.
func NewMessageStore(db *sql.DB) *MessageStore {
	return &MessageStore{db: db}
}

// Read returns every message for conversationID in write order. Unknown
// conversation → empty slice (matches memory.InMemoryStore). Malformed rows
// are skipped rather than failing the read, so one bad write can't poison
// the whole conversation.
func (s *MessageStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if conversationID == "" {
		return nil, fmt.Errorf("sqlite: invalid conversation id %q", conversationID)
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT message FROM messages WHERE conversation_id = ? ORDER BY seq`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("sqlite: read messages: %w", err)
	}
	defer rows.Close()

	out := make([]chat.Message, 0)
	for rows.Next() {
		var blob string
		if err := rows.Scan(&blob); err != nil {
			return nil, fmt.Errorf("sqlite: scan message: %w", err)
		}
		msg, err := chat.UnmarshalMessage([]byte(blob))
		if err != nil {
			continue
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: read messages: %w", err)
	}
	return out, nil
}

// Write appends messages to the conversation in one transaction. No-op for
// an empty batch; nil entries are skipped.
func (s *MessageStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if conversationID == "" {
		return fmt.Errorf("sqlite: invalid conversation id %q", conversationID)
	}
	if len(messages) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin write messages: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // commit overrides; rollback on early return

	for _, msg := range messages {
		if msg == nil {
			continue
		}
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("sqlite: marshal message: %w", err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO messages(conversation_id, message) VALUES (?, ?)`,
			conversationID, string(data),
		); err != nil {
			return fmt.Errorf("sqlite: append message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("sqlite: commit messages: %w", err)
	}
	return nil
}

// Clear drops every message for conversationID. Idempotent — unknown id is
// not an error (matches memory.InMemoryStore).
func (s *MessageStore) Clear(ctx context.Context, conversationID string) error {
	if conversationID == "" {
		return fmt.Errorf("sqlite: invalid conversation id %q", conversationID)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM messages WHERE conversation_id = ?`, conversationID,
	); err != nil {
		return fmt.Errorf("sqlite: clear messages: %w", err)
	}
	return nil
}
