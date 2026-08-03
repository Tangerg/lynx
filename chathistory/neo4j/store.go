package neo4j

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	DefaultDatabase = "neo4j"
	DefaultLabel    = "ChatMessage"

	fieldConversationID = "conversation_id"
	fieldMessage        = "message"
	fieldSequence       = "seq"
	parameterRows       = "rows"
)

// Config configures [New]. Only [Config.Driver] is required.
type Config struct {
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

var (
	_ chathistory.Store  = (*Store)(nil)
	_ chathistory.Lister = (*Store)(nil)
)

// Store is a Neo4j-backed [chathistory.Store]. Construct via [New].
type Store struct {
	driver   neo4j.DriverWithContext
	database string
	label    string
	sequence sequenceGenerator
}

// New builds a [Store] from cfg. ctx bounds optional index initialization.
func New(ctx context.Context, cfg Config) (*Store, error) {
	if isNilCapability(cfg.Driver) {
		return nil, errors.New("neo4j: driver is required")
	}
	if cfg.Database == "" {
		cfg.Database = DefaultDatabase
	}
	if cfg.Label == "" {
		cfg.Label = DefaultLabel
	}
	if !validIdentifier(cfg.Label) {
		return nil, fmt.Errorf("neo4j: label %q must be a valid unquoted identifier", cfg.Label)
	}
	s := &Store{
		driver:   cfg.Driver,
		database: cfg.Database,
		label:    cfg.Label,
	}
	if cfg.InitializeSchema {
		if err := s.initIndex(ctx); err != nil {
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

// Write creates a new node per message under conversationID. A reserved
// sequence range preserves argument order and remains monotonic if the local
// clock moves backward.
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

	encoded, err := encodeMessages(messages)
	if err != nil {
		return fmt.Errorf("neo4j: write: encode messages: %w", err)
	}
	sequenceBase := s.sequence.Reserve(len(encoded))
	rows := make([]map[string]any, 0, len(encoded))
	for index, raw := range encoded {
		rows = append(rows, map[string]any{
			fieldConversationID: conversationID,
			fieldSequence:       sequenceBase + int64(index),
			fieldMessage:        string(raw),
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
		map[string]any{parameterRows: rows},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return fmt.Errorf("neo4j: write: create message nodes: %w", err)
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order (seq ascending).
func (s *Store) Read(ctx context.Context, conversationID string) (storedMessages []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return nil, err
	}

	cypher := fmt.Sprintf(
		"MATCH (m:%s {conversation_id: $conversation_id}) RETURN elementId(m) AS id, m.seq AS seq, m.message AS message ORDER BY seq ASC, id ASC",
		s.label,
	)
	var result *neo4j.EagerResult
	result, err = neo4j.ExecuteQuery(ctx, s.driver, cypher,
		map[string]any{fieldConversationID: conversationID},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return nil, fmt.Errorf("neo4j: read: query messages: %w", err)
	}

	storedMessages = make([]chat.Message, 0, len(result.Records))
	for index, record := range result.Records {
		rawSequence, ok := record.Get(fieldSequence)
		if !ok {
			return nil, fmt.Errorf("neo4j: read: record %d is missing sequence", index)
		}
		sequence, ok := rawSequence.(int64)
		if !ok || sequence <= 0 {
			return nil, fmt.Errorf("neo4j: read: record %d sequence is %v (%T), want a positive integer", index, rawSequence, rawSequence)
		}
		raw, ok := record.Get(fieldMessage)
		if !ok {
			return nil, fmt.Errorf("neo4j: read: record %d is missing message", index)
		}
		encodedMessage, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("neo4j: read: record %d message has type %T, want string", index, raw)
		}
		message, err := decodeMessage([]byte(encodedMessage))
		if err != nil {
			return nil, fmt.Errorf("neo4j: read: decode message %d: %w", index, err)
		}
		storedMessages = append(storedMessages, message)
	}
	return storedMessages, nil
}

// Clear deletes every node for conversationID under the configured
// label. Unknown ids are a no-op.
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}

	cypher := fmt.Sprintf(
		"MATCH (m:%s {conversation_id: $conversation_id}) DELETE m",
		s.label,
	)
	_, err = neo4j.ExecuteQuery(ctx, s.driver, cypher,
		map[string]any{fieldConversationID: conversationID},
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return fmt.Errorf("neo4j: clear: delete message nodes: %w", err)
	}
	return nil
}

// Conversations returns every stored conversation ID in lexical order.
func (s *Store) Conversations(ctx context.Context) (ids []string, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	cypher := fmt.Sprintf(
		"MATCH (m:%s) RETURN DISTINCT m.conversation_id AS id",
		s.label,
	)
	var result *neo4j.EagerResult
	result, err = neo4j.ExecuteQuery(ctx, s.driver, cypher, nil,
		neo4j.EagerResultTransformer,
		neo4j.ExecuteQueryWithDatabase(s.database),
	)
	if err != nil {
		return nil, fmt.Errorf("neo4j: list conversations: query IDs: %w", err)
	}

	ids = make([]string, 0, len(result.Records))
	for index, record := range result.Records {
		raw, ok := record.Get("id")
		if !ok {
			return nil, fmt.Errorf("neo4j: list conversations: record %d is missing ID", index)
		}
		id, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("neo4j: list conversations: record %d ID has type %T, want string", index, raw)
		}
		if err := chathistory.ValidateConversationID(id); err != nil {
			return nil, fmt.Errorf("neo4j: list conversations: record %d has invalid ID %q: %w", index, id, err)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}
