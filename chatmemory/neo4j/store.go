package neo4j

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/Tangerg/lynx/chatmemory/internal/codec"
	"github.com/Tangerg/lynx/chatmemory/internal/tracing"
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

func (c StoreConfig) Validate() error {
	if c.Driver == nil {
		return errors.New("neo4j: Driver is required")
	}
	if !identPattern.MatchString(c.Label) {
		return fmt.Errorf("neo4j: Label=%q must match %s", c.Label, identPattern)
	}
	return nil
}

// ApplyDefaults fills zero fields. Context defaults to
// [context.Background]; Database defaults to [DefaultDatabase];
// Label defaults to [DefaultLabel].
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Database == "" {
		c.Database = DefaultDatabase
	}
	if c.Label == "" {
		c.Label = DefaultLabel
	}
}

var _ memory.Store = (*Store)(nil)

// Store is a Neo4j-backed [memory.Store]. Construct via [NewStore].
type Store struct {
	driver   neo4j.DriverWithContext
	database string
	label    string
}

// NewStore builds a [Store] from cfg.
func NewStore(cfg StoreConfig) (*Store, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
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
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	ctx, span := tracing.StartWrite(ctx, "neo4j", conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	now := time.Now().UnixNano()
	rows := make([]map[string]any, 0, len(messages))
	for i, msg := range messages {
		raw, encErr := codec.EncodeMessage(msg)
		if encErr != nil {
			err = fmt.Errorf("neo4j.Store.Write: encode message: %w", encErr)
			return err
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

	_, err = neo4j.ExecuteQuery(ctx, s.driver, cypher,
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
func (s *Store) Read(ctx context.Context, conversationID string) (out []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartRead(ctx, "neo4j", conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(out)) }()

	cypher := fmt.Sprintf(
		"MATCH (m:%s {conversation_id: $conversation_id}) RETURN m.message AS message ORDER BY m.seq ASC",
		s.label,
	)
	var result *neo4j.EagerResult
	result, err = neo4j.ExecuteQuery(ctx, s.driver, cypher,
		map[string]any{"conversation_id": conversationID},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return nil, fmt.Errorf("neo4j.Store.Read: %w", err)
	}

	out = make([]chat.Message, 0, len(result.Records))
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
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, "neo4j", conversationID)
	defer func() { tracing.Finish(span, err) }()

	cypher := fmt.Sprintf(
		"MATCH (m:%s {conversation_id: $conversation_id}) DELETE m",
		s.label,
	)
	_, err = neo4j.ExecuteQuery(ctx, s.driver, cypher,
		map[string]any{"conversation_id": conversationID},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return fmt.Errorf("neo4j.Store.Clear: %w", err)
	}
	return nil
}
