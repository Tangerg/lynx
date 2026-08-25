package redis

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

const DefaultKeyPrefix = "chat:history:"

var scanPatternEscaper = strings.NewReplacer(
	`\`, `\\`,
	`*`, `\*`,
	`?`, `\?`,
	`[`, `\[`,
	`]`, `\]`,
)

// Config configures [New]. Only [Config.Client] is required.
type Config struct {
	// Client is the live go-redis client. Required. The store does
	// not take ownership — callers Close() the client themselves.
	Client goredis.UniversalClient

	// KeyPrefix is prepended to every conversation id to namespace the
	// keys. Optional: defaults to [DefaultKeyPrefix].
	KeyPrefix string

	// TTL, when non-zero, applies a millisecond-precision expiry to every
	// conversation key and refreshes it on each Write. Zero means "never
	// expire".
	TTL time.Duration
}

var (
	_ chathistory.Store  = (*Store)(nil)
	_ chathistory.Lister = (*Store)(nil)
)

// Store is a Redis-backed [chathistory.Store]. Construct via [New].
type Store struct {
	client    goredis.UniversalClient
	keyPrefix string
	ttl       time.Duration
}

// New builds a [Store] from cfg.
func New(cfg Config) (*Store, error) {
	if isNilCapability(cfg.Client) {
		return nil, errors.New("redis: client is required")
	}
	if cfg.TTL < 0 {
		return nil, errors.New("redis: TTL must not be negative")
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = DefaultKeyPrefix
	}
	return &Store{
		client:    cfg.Client,
		keyPrefix: cfg.KeyPrefix,
		ttl:       cfg.TTL,
	}, nil
}

// key returns the namespaced Redis key for a conversation id.
func (s *Store) key(conversationID chathistory.ConversationID) string {
	return s.keyPrefix + conversationID.String()
}

// Write appends every message under conversationID. When TTL is set, append
// and expiry refresh execute in one Redis transaction. Empty writes are a
// no-op.
func (s *Store) Write(ctx context.Context, conversationID chathistory.ConversationID, messages ...chat.Message) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = conversationID.Validate(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	encoded, err := encodeMessages(messages)
	if err != nil {
		return fmt.Errorf("redis: write: encode messages: %w", err)
	}
	payloads := make([]any, len(encoded))
	for index, raw := range encoded {
		payloads[index] = raw
	}

	key := s.key(conversationID)
	if s.ttl == 0 {
		if err = s.client.RPush(ctx, key, payloads...).Err(); err != nil {
			return fmt.Errorf("redis: write: append messages: %w", err)
		}
		return nil
	}
	transaction := s.client.TxPipeline()
	transaction.RPush(ctx, key, payloads...)
	transaction.PExpire(ctx, key, s.ttl)
	if _, err = transaction.Exec(ctx); err != nil {
		return fmt.Errorf("redis: write: append messages and refresh expiry: %w", err)
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order. An empty slice is returned for unknown ids.
func (s *Store) Read(ctx context.Context, conversationID chathistory.ConversationID) (storedMessages []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = conversationID.Validate(); err != nil {
		return nil, err
	}

	var encodedMessages []string
	encodedMessages, err = s.client.LRange(ctx, s.key(conversationID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis: read: fetch messages: %w", err)
	}

	storedMessages = make([]chat.Message, 0, len(encodedMessages))
	for index, raw := range encodedMessages {
		message, err := decodeMessage([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("redis: read: decode message %d: %w", index, err)
		}
		storedMessages = append(storedMessages, message)
	}
	return storedMessages, nil
}

// Clear drops the entire list for conversationID. Unknown ids are
// silently ignored (DEL on a missing key is a no-op in Redis).
func (s *Store) Clear(ctx context.Context, conversationID chathistory.ConversationID) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = conversationID.Validate(); err != nil {
		return err
	}

	if err = s.client.Del(ctx, s.key(conversationID)).Err(); err != nil {
		return fmt.Errorf("redis: clear: delete conversation: %w", err)
	}
	return nil
}

// Conversations enumerates stored conversation IDs via SCAN and returns them
// in lexical order. SCAN may observe concurrent mutations and repeat keys, so
// results are de-duplicated.
func (s *Store) Conversations(ctx context.Context) (ids []chathistory.ConversationID, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	match := scanPatternEscaper.Replace(s.keyPrefix) + "*"
	seen := make(map[string]struct{})
	// Non-nil even when no conversations exist — every backend's
	// Conversations returns an empty slice, not nil.
	ids = []chathistory.ConversationID{}

	var cursor uint64
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}

		var keys []string
		keys, cursor, err = s.client.Scan(ctx, cursor, match, 0).Result()
		if err != nil {
			return nil, fmt.Errorf("redis: list conversations: scan keys: %w", err)
		}

		for _, key := range keys {
			id, ok := strings.CutPrefix(key, s.keyPrefix)
			if !ok {
				// MATCH should preclude this, but guard against the
				// prefix incidentally matching unintended keys.
				continue
			}
			conversationID := chathistory.ConversationID(id)
			if conversationID.Validate() != nil {
				continue
			}
			if _, duplicate := seen[id]; duplicate {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, conversationID)
		}

		if cursor == 0 {
			break
		}
	}
	slices.Sort(ids)
	return ids, nil
}
