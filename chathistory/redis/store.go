package redis

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/internal/codec"
	"github.com/Tangerg/lynx/chathistory/internal/tracing"
	"github.com/Tangerg/lynx/core/chat"
)

const DefaultKeyPrefix = "chat:history:"

// Config configures [New]. Only [Config.Client] is required.
type Config struct {
	// Client is the live go-redis client. Required. The store does
	// not take ownership — callers Close() the client themselves.
	Client goredis.UniversalClient

	// KeyPrefix is prepended to every conversation id to namespace the
	// keys. Optional: defaults to [DefaultKeyPrefix].
	KeyPrefix string

	// TTL, when non-zero, applies an expiry to every conversation
	// key (set on Write; refreshed on each subsequent Write). Zero
	// means "never expire".
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
	if cfg.Client == nil {
		return nil, errors.New("redis: Client is required")
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
func (s *Store) key(conversationID string) string {
	return s.keyPrefix + conversationID
}

// Write RPUSH'es every message under conversationID. When TTL is set
// the key's expiry is refreshed in the same pipeline. No-op when
// messages is empty.
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	ctx, span := tracing.StartWrite(ctx, "redis", conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	payloads := make([]any, 0, len(messages))
	for _, msg := range messages {
		raw, encErr := codec.EncodeMessage(msg)
		if encErr != nil {
			return fmt.Errorf("redis.Store.Write: encode message: %w", encErr)
		}
		payloads = append(payloads, raw)
	}

	key := s.key(conversationID)
	pipe := s.client.Pipeline()
	pipe.RPush(ctx, key, payloads...)
	if s.ttl > 0 {
		pipe.Expire(ctx, key, s.ttl)
	}
	if _, err = pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis.Store.Write: %w", err)
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order. An empty slice is returned for unknown ids.
func (s *Store) Read(ctx context.Context, conversationID string) (out []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartRead(ctx, "redis", conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(out)) }()

	var raws []string
	raws, err = s.client.LRange(ctx, s.key(conversationID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis.Store.Read: %w", err)
	}

	out = make([]chat.Message, 0, len(raws))
	for _, raw := range raws {
		msg, err := codec.DecodeMessage([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("redis.Store.Read: decode message: %w", err)
		}
		out = append(out, msg)
	}
	return out, nil
}

// Clear drops the entire list for conversationID. Unknown ids are
// silently ignored (DEL on a missing key is a no-op in Redis).
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, "redis", conversationID)
	defer func() { tracing.Finish(span, err) }()

	if err = s.client.Del(ctx, s.key(conversationID)).Err(); err != nil {
		return fmt.Errorf("redis.Store.Clear: %w", err)
	}
	return nil
}

// Conversations enumerates the ids of every stored conversation via a
// non-blocking SCAN over the keyPrefix namespace. The returned slice is
// a point-in-time snapshot in no guaranteed order; SCAN may surface a
// given key more than once across cursor iterations, so ids are
// de-duplicated. Honors ctx cancellation.
func (s *Store) Conversations(ctx context.Context) (ids []string, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartList(ctx, "redis")
	defer func() { tracing.RecordListResult(span, err, len(ids)) }()

	match := s.keyPrefix + "*"
	seen := make(map[string]struct{})
	// Non-nil even when no conversations exist — every backend's
	// Conversations returns an empty slice, not nil.
	ids = []string{}

	var cursor uint64
	for {
		if err = ctx.Err(); err != nil {
			return nil, err
		}

		var keys []string
		keys, cursor, err = s.client.Scan(ctx, cursor, match, 0).Result()
		if err != nil {
			return nil, fmt.Errorf("redis.Store.Conversations: %w", err)
		}

		for _, k := range keys {
			id, ok := strings.CutPrefix(k, s.keyPrefix)
			if !ok {
				// MATCH should preclude this, but guard against the
				// prefix incidentally matching unintended keys.
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}

		if cursor == 0 {
			break
		}
	}
	return ids, nil
}
