package cassandra

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gocql/gocql"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/history"
)

const (
	DefaultKeyspace  = "scope"
	DefaultTableName = "chat_history"
)

// StoreConfig configures [NewStore]. Only [StoreConfig.Session] is required.
type StoreConfig struct {
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

// Validate reports whether c can be used to construct a [Store]. Blank
// optional identifiers are valid and are resolved to their documented
// defaults by [NewStore].
func (c StoreConfig) Validate() error {
	if c.Session == nil {
		return errors.New("cassandra: session is required")
	}
	if c.Keyspace != "" && !validIdentifier(c.Keyspace) {
		return fmt.Errorf("cassandra: keyspace %q must be a valid unquoted identifier", c.Keyspace)
	}
	if c.TableName != "" && !validIdentifier(c.TableName) {
		return fmt.Errorf("cassandra: table name %q must be a valid unquoted identifier", c.TableName)
	}
	return nil
}

var (
	_ history.Store  = (*Store)(nil)
	_ history.Lister = (*Store)(nil)
)

// Store is a Cassandra-backed [history.Store]. Construct via [NewStore].
type Store struct {
	session  *gocql.Session
	sequence sequenceGenerator

	writeCQL  string
	readCQL   string
	clearCQL  string
	listCQL   string
	createCQL string
}

// New builds a [Store] from cfg. ctx bounds optional table initialization.
func NewStore(ctx context.Context, cfg StoreConfig) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if cfg.Keyspace == "" {
		cfg.Keyspace = DefaultKeyspace
	}
	if cfg.TableName == "" {
		cfg.TableName = DefaultTableName
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
func (s *Store) Write(ctx context.Context, conversationID history.ConversationID, messages ...chat.Message) (err error) {
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
		return fmt.Errorf("cassandra: write: encode messages: %w", err)
	}
	batch := s.session.NewBatch(gocql.UnloggedBatch).WithContext(ctx)
	sequenceBase := s.sequence.Reserve(len(encoded) * 100)
	for index, raw := range encoded {
		messageSequence := sequenceUUID(sequenceBase, index)
		batch.Query(s.writeCQL, conversationID.String(), messageSequence, string(raw))
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
func (s *Store) Read(ctx context.Context, conversationID history.ConversationID) (storedMessages []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = conversationID.Validate(); err != nil {
		return nil, err
	}

	iterator := s.session.Query(s.readCQL, conversationID.String()).WithContext(ctx).Iter()
	defer closeIterator(iterator, "read", &err)

	storedMessages = []chat.Message{}
	var encodedMessage string
	for iterator.Scan(&encodedMessage) {
		message, decodeErr := decodeMessage([]byte(encodedMessage))
		if decodeErr != nil {
			err = fmt.Errorf("cassandra: read: decode message %d: %w", len(storedMessages), decodeErr)
			return nil, err
		}
		storedMessages = append(storedMessages, message)
	}
	return storedMessages, nil
}

// Clear drops every row for conversationID. Unknown ids are a no-op.
func (s *Store) Clear(ctx context.Context, conversationID history.ConversationID) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = conversationID.Validate(); err != nil {
		return err
	}

	if err = s.session.Query(s.clearCQL, conversationID.String()).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("cassandra: clear: delete partition: %w", err)
	}
	return nil
}

// Conversations returns every stored conversation ID in lexical order.
//
// SELECT DISTINCT on the partition key reads only partition metadata,
// so no ALLOW FILTERING is needed.
func (s *Store) Conversations(ctx context.Context) (ids []history.ConversationID, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	iterator := s.session.Query(s.listCQL).WithContext(ctx).Iter()
	defer closeIterator(iterator, "list conversations", &err)

	ids = []history.ConversationID{}
	var id string
	for iterator.Scan(&id) {
		conversationID := history.ConversationID(id)
		if err := conversationID.Validate(); err != nil {
			return nil, fmt.Errorf("cassandra: list conversations: invalid stored ID %q: %w", id, err)
		}
		ids = append(ids, conversationID)
	}
	slices.Sort(ids)
	return ids, nil
}

func closeIterator(iterator *gocql.Iter, operation string, operationErr *error) {
	if err := iterator.Close(); err != nil {
		*operationErr = errors.Join(*operationErr, fmt.Errorf("cassandra: %s: close iterator: %w", operation, err))
	}
}
