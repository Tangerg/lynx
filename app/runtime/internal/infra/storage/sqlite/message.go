package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/history"
)

// errEmptyConversationID guards every store operation: the conversation id is
// the table key, so an empty one is a caller bug, not an empty conversation.
var errEmptyConversationID = errors.New("sqlite: conversation id is required")

// MessageStore implements the lynx-core chat history [history.Store] against
// SQLite — the per-conversation chat history the history middleware loads
// before each turn and appends to after. One append-only table keyed by
// conversation, ordered by an autoincrement seq; each [chat.Message] is
// stored as opaque JSON (round-tripped via [chat.UnmarshalMessage]).
//
// Append-only: one INSERT per message — O(1) writes, ordered reads, no
// whole-file rewrite.
type MessageStore struct {
	db *sql.DB
}

var (
	_ history.Store    = (*MessageStore)(nil)
	_ history.Replacer = (*MessageStore)(nil)
	_ history.Counter  = (*MessageStore)(nil)
)

// NewMessageStore binds the chat history store to a database opened via [Open].
func NewMessageStore(db *sql.DB) *MessageStore {
	return &MessageStore{db: db}
}

// Read returns every message for conversationID in write order. Unknown
// conversation → empty slice (matches history.InMemoryStore). Malformed rows
// are skipped rather than failing the read, so one bad write can't poison
// the whole conversation.
func (s *MessageStore) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if conversationID == "" {
		return nil, errEmptyConversationID
	}
	rows, err := conn(ctx, s.db).QueryContext(ctx,
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
		return errEmptyConversationID
	}
	if len(messages) == 0 {
		return nil
	}
	// RunInTx so the batch is atomic standalone, and folds into a caller's
	// cross-store transaction (sessions.import seeds history inside one) instead
	// of opening its own — which would deadlock under MaxOpenConns(1).
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		for _, msg := range messages {
			if msg == nil {
				continue
			}
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
// [history.Replacer] contract). Empty messages clears the conversation.
// Retention (truncate / compaction) uses this instead of Clear+Write, which
// would lose the conversation if the Write failed after the Clear committed.
func (s *MessageStore) Replace(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if conversationID == "" {
		return errEmptyConversationID
	}
	return RunInTx(ctx, s.db, func(ctx context.Context) error {
		q := conn(ctx, s.db)
		if _, err := q.ExecContext(ctx,
			`DELETE FROM messages WHERE conversation_id = ?`, conversationID,
		); err != nil {
			return fmt.Errorf("sqlite: replace clear messages: %w", err)
		}
		for _, msg := range messages {
			if msg == nil {
				continue
			}
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
// [history.Counter] capability — so a watermark read (sessions.rollback /
// fork{fromRunId}) doesn't load and unmarshal the whole history just to take
// its length. Unknown conversation → 0. COUNT(*) tallies stored rows; Read
// skips any that fail to unmarshal, but Write only persists marshalable
// messages, so in practice the two agree.
func (s *MessageStore) Count(ctx context.Context, conversationID string) (int, error) {
	if conversationID == "" {
		return 0, errEmptyConversationID
	}
	var n int
	if err := conn(ctx, s.db).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages WHERE conversation_id = ?`, conversationID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("sqlite: count messages: %w", err)
	}
	return n, nil
}

// Clear drops every message for conversationID. Idempotent — unknown id is
// not an error (matches history.InMemoryStore).
func (s *MessageStore) Clear(ctx context.Context, conversationID string) error {
	if conversationID == "" {
		return errEmptyConversationID
	}
	if _, err := conn(ctx, s.db).ExecContext(ctx,
		`DELETE FROM messages WHERE conversation_id = ?`, conversationID,
	); err != nil {
		return fmt.Errorf("sqlite: clear messages: %w", err)
	}
	return nil
}
