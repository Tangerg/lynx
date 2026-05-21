package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/Tangerg/lynx/chatmemory/internal/tracing"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

const Provider = "RedisChatMemory"

const DefaultKeyPrefix = "chat:memory:"

// StoreConfig configures [NewStore]. Only [StoreConfig.Client] is
// required.
type StoreConfig struct {
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

func (c *StoreConfig) validate() error {
	if c == nil {
		return errors.New("redis: config must not be nil")
	}
	if c.Client == nil {
		return errors.New("redis: Client is required")
	}
	if c.KeyPrefix == "" {
		c.KeyPrefix = DefaultKeyPrefix
	}
	if c.TTL < 0 {
		return errors.New("redis: TTL must not be negative")
	}
	return nil
}

var _ memory.Store = (*Store)(nil)

// Store is a Redis-backed [memory.Store]. Construct via [NewStore].
type Store struct {
	client    goredis.UniversalClient
	keyPrefix string
	ttl       time.Duration
}

// NewStore builds a [Store] from cfg.
func NewStore(cfg *StoreConfig) (*Store, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
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
	if len(messages) == 0 {
		return nil
	}

	ctx, span := tracing.StartWrite(ctx, "redis", conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	payloads := make([]any, 0, len(messages))
	for _, msg := range messages {
		raw, err := encodeMessage(msg)
		if err != nil {
			return fmt.Errorf("redis.Store.Write: encode message: %w", err)
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

	ctx, span := tracing.StartRead(ctx, "redis", conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(out)) }()

	var raws []string
	raws, err = s.client.LRange(ctx, s.key(conversationID), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis.Store.Read: %w", err)
	}

	out = make([]chat.Message, 0, len(raws))
	for _, raw := range raws {
		msg, err := chat.UnmarshalMessage([]byte(raw))
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

	ctx, span := tracing.StartClear(ctx, "redis", conversationID)
	defer func() { tracing.Finish(span, err) }()

	if err = s.client.Del(ctx, s.key(conversationID)).Err(); err != nil {
		return fmt.Errorf("redis.Store.Clear: %w", err)
	}
	return nil
}

// encodeMessage marshals msg via its MarshalJSON. See the postgres
// provider for the rationale on the per-type switch.
func encodeMessage(msg chat.Message) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("message must not be nil")
	}
	switch m := msg.(type) {
	case *chat.SystemMessage:
		return m.MarshalJSON()
	case *chat.UserMessage:
		return m.MarshalJSON()
	case *chat.AssistantMessage:
		return m.MarshalJSON()
	case *chat.ToolMessage:
		return m.MarshalJSON()
	default:
		return nil, fmt.Errorf("unsupported message type %T", msg)
	}
}
