package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/Tangerg/scope/core/chat"
	history "github.com/Tangerg/scope/core/history"
)

// MessageStore persists the Runtime's per-session model-context history in
// SQLite. One append-only table is keyed by session and ordered by an
// autoincrement seq; each [chat.Message] is stored as opaque JSON
// (round-tripped via [chat.UnmarshalMessage]).
//
// Append-only: one INSERT per message — O(1) writes, ordered reads, no
// whole-file rewrite.
type MessageStore struct {
	db *sql.DB
}

// NewMessageStore binds the chat history store to a database opened via [Open].
func NewMessageStore(db *sql.DB) *MessageStore {
	return &MessageStore{db: db}
}

// Read returns every message for conversationID in write order. Unknown
// conversation → empty slice (matches in-memory history store). Malformed rows
// are skipped rather than failing the read, so one bad write can't poison
// the whole conversation.
func (m *MessageStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := history.ConversationID(conversationID).Validate(); err != nil {
		return nil, err
	}
	rows, err := conn(ctx, m.db).QueryContext(ctx,
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
		var msg chat.Message
		if err := json.Unmarshal([]byte(blob), &msg); err != nil {
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
// an empty batch.
func (m *MessageStore) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := history.ConversationID(conversationID).Validate(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}
	// RunInTx so the batch is atomic standalone, and folds into a caller's
	// cross-store transaction (portable restore seeds history inside one) instead
	// of opening its own — which would deadlock under MaxOpenConns(1).
	return RunInTx(ctx, m.db, func(ctx context.Context) error {
		q := conn(ctx, m.db)
		for _, msg := range messages {
			data, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("sqlite: marshal message: %w", err)
			}
			if _, err := q.ExecContext(ctx,
				`INSERT INTO messages(conversation_id, message) VALUES (?, ?)`,
				conversationID, string(data),
			); err != nil {
				return fmt.Errorf("sqlite: append message: %w", err)
			}
		}
		return nil
	})
}

// Replace atomically sets conversationID's history to exactly messages — a
// single transaction that DELETEs the existing rows then INSERTs the new ones,
// so a failed rewrite rolls back and leaves the prior history intact (the
// consumer's atomic-replacement contract). Empty messages clears the
// conversation.
// Retention (truncate / compaction) uses this instead of Clear+Write, which
// would lose the conversation if the Write failed after the Clear committed.
func (m *MessageStore) Replace(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := history.ConversationID(conversationID).Validate(); err != nil {
		return err
	}
	return RunInTx(ctx, m.db, func(ctx context.Context) error {
		q := conn(ctx, m.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM messages WHERE conversation_id = ?`, conversationID,
		); err != nil {
			return fmt.Errorf("sqlite: replace clear messages: %w", err)
		}
		for _, msg := range messages {
			data, err := json.Marshal(msg)
			if err != nil {
				return fmt.Errorf("sqlite: marshal message: %w", err)
			}
			if _, err := q.ExecContext(ctx,
				`INSERT INTO messages(conversation_id, message) VALUES (?, ?)`,
				conversationID, string(data),
			); err != nil {
				return fmt.Errorf("sqlite: replace append message: %w", err)
			}
		}
		return nil
	})
}

// Count returns conversationID's message count via a COUNT(*) query — the
// conversation use case, so a rollback/fork watermark read
// fork{fromRunId}) doesn't load and unmarshal the whole history just to take
// its length. Unknown conversation → 0. COUNT(*) tallies stored rows; Read
// skips any that fail to unmarshal, but Write only persists marshalable
// messages, so in practice the two agree.
func (m *MessageStore) Count(ctx context.Context, conversationID string) (int, error) {
	if err := history.ConversationID(conversationID).Validate(); err != nil {
		return 0, err
	}
	var n int
	if err := conn(ctx, m.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE conversation_id = ?`, conversationID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: count messages: %w", err)
	}
	return n, nil
}

// Clear drops every message for conversationID. Idempotent — unknown id is
// not an error (matches in-memory history store).
func (m *MessageStore) Clear(ctx context.Context, conversationID string) error {
	if err := history.ConversationID(conversationID).Validate(); err != nil {
		return err
	}
	if _, err := conn(ctx, m.db).ExecContext(ctx,
		`DELETE FROM messages WHERE conversation_id = ?`, conversationID,
	); err != nil {
		return fmt.Errorf("sqlite: clear messages: %w", err)
	}
	return nil
}
