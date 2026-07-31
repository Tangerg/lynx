package cassandra

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gocql/gocql"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/internal/codec"
	"github.com/Tangerg/lynx/chathistory/internal/dbident"
	"github.com/Tangerg/lynx/chathistory/internal/sequence"
	"github.com/Tangerg/lynx/chathistory/internal/tracing"
	"github.com/Tangerg/lynx/core/chat"
)

const (
	DefaultKeyspace  = "lynx"
	DefaultTableName = "chat_history"
)

// Config configures [New]. Only [Config.Session] is required.
type Config struct {
	// Session is the live gocql session. Required. Callers own
	// session lifetime.
	Session *gocql.Session

	// Keyspace is the CQL keyspace. Optional: defaults to
	// [DefaultKeyspace]. The keyspace must already exist (Cassandra
	// keyspace creation needs replication-strategy choices the store
	// cannot make on the user's behalf).
	Keyspace string

	// TableName is the CQL table. Optional: defaults to
	// [DefaultTableName] ("chat_history").
	TableName string

	// InitializeSchema, when true, creates the table if it doesn't
	// already exist. The keyspace itself is NOT created.
	InitializeSchema bool
}

var (
	_ chathistory.Store  = (*Store)(nil)
	_ chathistory.Lister = (*Store)(nil)
)

// Store is a Cassandra-backed [chathistory.Store]. Construct via [New].
type Store struct {
	session  *gocql.Session
	sequence sequence.Generator

	writeCQL  string
	readCQL   string
	clearCQL  string
	listCQL   string
	createCQL string
}

// New builds a [Store] from cfg. ctx bounds optional table initialization.
func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Session == nil {
		return nil, errors.New("cassandra: session is required")
	}
	if cfg.Keyspace == "" {
		cfg.Keyspace = DefaultKeyspace
	}
	if cfg.TableName == "" {
		cfg.TableName = DefaultTableName
	}
	if !dbident.Valid(cfg.Keyspace) {
		return nil, fmt.Errorf("cassandra: keyspace %q must be a valid unquoted identifier", cfg.Keyspace)
	}
	if !dbident.Valid(cfg.TableName) {
		return nil, fmt.Errorf("cassandra: table name %q must be a valid unquoted identifier", cfg.TableName)
	}

	qualified := cfg.Keyspace + "." + cfg.TableName
	s := &Store{
		session: cfg.Session,
		writeCQL: fmt.Sprintf(
			"INSERT INTO %s (conversation_id, seq, message) VALUES (?, ?, ?)",
			qualified,
		),
		readCQL: fmt.Sprintf(
			"SELECT message FROM %s WHERE conversation_id = ? ORDER BY seq ASC",
			qualified,
		),
		clearCQL: fmt.Sprintf("DELETE FROM %s WHERE conversation_id = ?", qualified),
		listCQL:  fmt.Sprintf("SELECT DISTINCT conversation_id FROM %s", qualified),
		createCQL: fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			conversation_id TEXT,
			seq             TIMEUUID,
			message         TEXT,
			PRIMARY KEY ((conversation_id), seq)
		) WITH CLUSTERING ORDER BY (seq ASC)`, qualified),
	}

	if cfg.InitializeSchema {
		if err := s.session.Query(s.createCQL).WithContext(ctx).Exec(); err != nil {
			return nil, fmt.Errorf("cassandra: create table: %w", err)
		}
	}

	return s, nil
}

// Write appends every message under conversationID in one single-partition
// unlogged batch. Client-generated TIMEUUIDs are strictly increasing within
// one call; concurrent calls have no defined relative order.
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

	ctx, span := tracing.StartWrite(ctx, tracing.Cassandra, conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	encoded, err := codec.EncodeMessages(messages)
	if err != nil {
		return fmt.Errorf("cassandra: write: encode messages: %w", err)
	}
	batch := s.session.NewBatch(gocql.UnloggedBatch).WithContext(ctx)
	sequenceBase := s.sequence.Reserve(len(encoded) * 100)
	for index, raw := range encoded {
		messageSequence := sequenceUUID(sequenceBase, index)
		batch.Query(s.writeCQL, conversationID, messageSequence, string(raw))
	}
	if err = s.session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("cassandra: write: execute batch: %w", err)
	}
	return nil
}

func sequenceUUID(base int64, index int) gocql.UUID {
	return gocql.UUIDFromTime(time.Unix(0, base+int64(index)*100))
}

// Read returns every message stored under conversationID in
// insertion order (TIMEUUID ascending).
func (s *Store) Read(ctx context.Context, conversationID string) (storedMessages []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartRead(ctx, tracing.Cassandra, conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(storedMessages)) }()

	iterator := s.session.Query(s.readCQL, conversationID).WithContext(ctx).Iter()
	defer closeIterator(iterator, "read", &err)

	storedMessages = []chat.Message{}
	var encodedMessage string
	for iterator.Scan(&encodedMessage) {
		message, decodeErr := codec.DecodeMessage([]byte(encodedMessage))
		if decodeErr != nil {
			err = fmt.Errorf("cassandra: read: decode message %d: %w", len(storedMessages), decodeErr)
			return nil, err
		}
		storedMessages = append(storedMessages, message)
	}
	return storedMessages, nil
}

// Clear drops every row for conversationID. Unknown ids are a no-op.
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, tracing.Cassandra, conversationID)
	defer func() { tracing.Finish(span, err) }()

	if err = s.session.Query(s.clearCQL, conversationID).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("cassandra: clear: delete partition: %w", err)
	}
	return nil
}

// Conversations returns every stored conversation ID in lexical order.
//
// SELECT DISTINCT on the partition key reads only partition metadata,
// so no ALLOW FILTERING is needed.
func (s *Store) Conversations(ctx context.Context) (ids []string, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartList(ctx, tracing.Cassandra)
	defer func() { tracing.RecordListResult(span, err, len(ids)) }()

	iterator := s.session.Query(s.listCQL).WithContext(ctx).Iter()
	defer closeIterator(iterator, "list conversations", &err)

	ids = []string{}
	var id string
	for iterator.Scan(&id) {
		if err := chathistory.ValidateConversationID(id); err != nil {
			return nil, fmt.Errorf("cassandra: list conversations: invalid stored ID %q: %w", id, err)
		}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids, nil
}

func closeIterator(iterator *gocql.Iter, operation string, operationErr *error) {
	if err := iterator.Close(); err != nil {
		*operationErr = errors.Join(*operationErr, fmt.Errorf("cassandra: %s: close iterator: %w", operation, err))
	}
}
