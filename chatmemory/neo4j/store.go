// Package neo4j is a [memory.Store] backed by Neo4j via the official
// Go driver (v5).
//
// Storage model:
//
//	(:ChatMessage {
//	    conversation_id: "u-42",
//	    seq:             <int64 nanos>,
//	    message:         "<json>",
//	    created_at:      <datetime>
//	})
//
// A composite index on (`conversation_id`, `seq`) is created by
// InitializeSchema=true so reads stream in insertion order without a
// full collection scan. `seq` is a Go-side nanosecond timestamp; the
// batch-offset is added to ensure messages from one Write call are
// strictly ordered even when nanoseconds happen to collide.
//
// Example:
//
//	drv, _ := neo4j.NewDriverWithContext("neo4j://...", auth)
//	defer drv.Close(ctx)
//	store, _ := neo4jmem.NewStore(&neo4jmem.StoreConfig{
//	    Driver:           drv,
//	    Database:         "neo4j",
//	    InitializeSchema: true,
//	})
package neo4j

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

const Provider = "Neo4jChatMemory"

const (
	DefaultDatabase = "neo4j"
	DefaultLabel    = "ChatMessage"
)

// identPattern restricts the user-supplied Cypher node label / index
// name to the conservative shape Neo4j accepts without quoting.
var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// StoreConfig configures [NewStore]. Only [StoreConfig.Driver] is
// required.
type StoreConfig struct {
	// Context is used for the schema bootstrap. Optional: defaults
	// to context.Background().
	Context context.Context

	// Driver is the live Neo4j driver. Required. Callers own its
	// lifetime.
	Driver neo4j.DriverWithContext

	// Database selects the Neo4j database to operate against.
	// Optional: defaults to [DefaultDatabase] ("neo4j").
	Database string

	// Label is the node label used for stored messages. Optional:
	// defaults to [DefaultLabel] ("ChatMessage").
	Label string

	// InitializeSchema, when true, creates an index on
	// (conversation_id, seq) for the chosen label. Idempotent.
	InitializeSchema bool
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return errors.New("neo4j: config must not be nil")
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Driver == nil {
		return errors.New("neo4j: Driver is required")
	}
	if c.Database == "" {
		c.Database = DefaultDatabase
	}
	if c.Label == "" {
		c.Label = DefaultLabel
	}
	if !identPattern.MatchString(c.Label) {
		return fmt.Errorf("neo4j: Label=%q must match %s", c.Label, identPattern)
	}
	return nil
}

var _ memory.Store = (*Store)(nil)

// Store is a Neo4j-backed [memory.Store]. Construct via [NewStore].
type Store struct {
	driver   neo4j.DriverWithContext
	database string
	label    string
}

// NewStore builds a [Store] from cfg.
func NewStore(cfg *StoreConfig) (*Store, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	s := &Store{
		driver:   cfg.Driver,
		database: cfg.Database,
		label:    cfg.Label,
	}
	if cfg.InitializeSchema {
		if err := s.initIndex(cfg.Context); err != nil {
			return nil, fmt.Errorf("neo4j: initialize schema: %w", err)
		}
	}
	return s, nil
}

// initIndex creates the (conversation_id, seq) range index on the
// configured label. Idempotent.
func (s *Store) initIndex(ctx context.Context) error {
	indexName := s.label + "_conversation_seq_idx"
	cypher := fmt.Sprintf(
		"CREATE INDEX %s IF NOT EXISTS FOR (m:%s) ON (m.conversation_id, m.seq)",
		indexName, s.label,
	)
	_, err := neo4j.ExecuteQuery(ctx, s.driver, cypher, nil,
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	return err
}

// Write creates a new node per message under conversationID. `seq`
// is filled with `nowNanos + batchIndex` so all messages in one
// Write call sort strictly even on nanosecond-clock collisions.
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	now := time.Now().UnixNano()
	rows := make([]map[string]any, 0, len(messages))
	for i, msg := range messages {
		raw, err := encodeMessage(msg)
		if err != nil {
			return fmt.Errorf("neo4j.Store.Write: encode message: %w", err)
		}
		rows = append(rows, map[string]any{
			"conversation_id": conversationID,
			"seq":             now + int64(i),
			"message":         string(raw),
		})
	}

	cypher := fmt.Sprintf(`
		UNWIND $rows AS row
		CREATE (m:%s {
			conversation_id: row.conversation_id,
			seq:             row.seq,
			message:         row.message,
			created_at:      datetime()
		})`, s.label)

	_, err := neo4j.ExecuteQuery(ctx, s.driver, cypher,
		map[string]any{"rows": rows},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return fmt.Errorf("neo4j.Store.Write: %w", err)
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order (seq ascending).
func (s *Store) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cypher := fmt.Sprintf(
		"MATCH (m:%s {conversation_id: $conversation_id}) RETURN m.message AS message ORDER BY m.seq ASC",
		s.label,
	)
	result, err := neo4j.ExecuteQuery(ctx, s.driver, cypher,
		map[string]any{"conversation_id": conversationID},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return nil, fmt.Errorf("neo4j.Store.Read: %w", err)
	}

	out := make([]chat.Message, 0, len(result.Records))
	for _, rec := range result.Records {
		raw, ok := rec.Get("message")
		if !ok {
			continue
		}
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("neo4j.Store.Read: message column type %T, want string", raw)
		}
		msg, err := chat.UnmarshalMessage([]byte(s))
		if err != nil {
			return nil, fmt.Errorf("neo4j.Store.Read: decode message: %w", err)
		}
		out = append(out, msg)
	}
	return out, nil
}

// Clear deletes every node for conversationID under the configured
// label. Unknown ids are a no-op.
func (s *Store) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cypher := fmt.Sprintf(
		"MATCH (m:%s {conversation_id: $conversation_id}) DELETE m",
		s.label,
	)
	_, err := neo4j.ExecuteQuery(ctx, s.driver, cypher,
		map[string]any{"conversation_id": conversationID},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return fmt.Errorf("neo4j.Store.Clear: %w", err)
	}
	return nil
}

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
