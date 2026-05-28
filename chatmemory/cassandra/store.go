package cassandra

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/gocql/gocql"

	"github.com/Tangerg/lynx/chatmemory/internal/codec"
	"github.com/Tangerg/lynx/chatmemory/internal/tracing"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

const Provider = "CassandraChatMemory"

const (
	DefaultKeyspace  = "lynx"
	DefaultTableName = "chat_memory"
)

// identPattern matches valid Cassandra unquoted-identifier shape.
var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// StoreConfig configures [NewStore]. Only [StoreConfig.Session] is
// required.
type StoreConfig struct {
	// Context is used for the schema bootstrap when InitializeSchema
	// is true. Optional: defaults to context.Background().
	Context context.Context

	// Session is the live gocql session. Required. Callers own
	// session lifetime.
	Session *gocql.Session

	// Keyspace is the CQL keyspace. Optional: defaults to
	// [DefaultKeyspace]. The keyspace must already exist (Cassandra
	// keyspace creation needs replication-strategy choices the store
	// cannot make on the user's behalf).
	Keyspace string

	// TableName is the CQL table. Optional: defaults to
	// [DefaultTableName] ("chat_memory").
	TableName string

	// InitializeSchema, when true, creates the table if it doesn't
	// already exist. The keyspace itself is NOT created.
	InitializeSchema bool
}

func (c StoreConfig) Validate() error {
	if c.Session == nil {
		return errors.New("cassandra: Session is required")
	}
	for name, value := range map[string]string{"Keyspace": c.Keyspace, "TableName": c.TableName} {
		if !identPattern.MatchString(value) {
			return fmt.Errorf("cassandra: %s=%q must match %s", name, value, identPattern)
		}
	}
	return nil
}

// ApplyDefaults fills zero fields. Context defaults to
// [context.Background]; Keyspace defaults to [DefaultKeyspace];
// TableName defaults to [DefaultTableName].
func (c *StoreConfig) ApplyDefaults() {
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Keyspace == "" {
		c.Keyspace = DefaultKeyspace
	}
	if c.TableName == "" {
		c.TableName = DefaultTableName
	}
}

var _ memory.Store = (*Store)(nil)

// Store is a Cassandra-backed [memory.Store]. Construct via [NewStore].
type Store struct {
	session *gocql.Session

	writeCQL  string
	readCQL   string
	clearCQL  string
	createCQL string
}

// NewStore builds a [Store] from cfg.
func NewStore(cfg StoreConfig) (*Store, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	qualified := cfg.Keyspace + "." + cfg.TableName
	s := &Store{
		session: cfg.Session,
		writeCQL: fmt.Sprintf(
			"INSERT INTO %s (conversation_id, seq, message) VALUES (?, now(), ?)",
			qualified,
		),
		readCQL: fmt.Sprintf(
			"SELECT message FROM %s WHERE conversation_id = ? ORDER BY seq ASC",
			qualified,
		),
		clearCQL: fmt.Sprintf("DELETE FROM %s WHERE conversation_id = ?", qualified),
		createCQL: fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			conversation_id TEXT,
			seq             TIMEUUID,
			message         TEXT,
			PRIMARY KEY ((conversation_id), seq)
		) WITH CLUSTERING ORDER BY (seq ASC)`, qualified),
	}

	if cfg.InitializeSchema {
		if err := s.session.Query(s.createCQL).WithContext(cfg.Context).Exec(); err != nil {
			return nil, fmt.Errorf("cassandra: create table: %w", err)
		}
	}

	return s, nil
}

// Write appends every message under conversationID. Each insert
// stamps `seq = now()` server-side, yielding a globally-monotone
// TIMEUUID clustering key.
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	ctx, span := tracing.StartWrite(ctx, "cassandra", conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	for _, msg := range messages {
		raw, encErr := codec.EncodeMessage(msg)
		if encErr != nil {
			err = fmt.Errorf("cassandra.Store.Write: encode message: %w", encErr)
			return err
		}
		if err = s.session.Query(s.writeCQL, conversationID, string(raw)).WithContext(ctx).Exec(); err != nil {
			return fmt.Errorf("cassandra.Store.Write: %w", err)
		}
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order (TIMEUUID ascending).
func (s *Store) Read(ctx context.Context, conversationID string) (out []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartRead(ctx, "cassandra", conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(out)) }()

	iter := s.session.Query(s.readCQL, conversationID).WithContext(ctx).Iter()
	defer iter.Close()

	out = []chat.Message{}
	var raw string
	for iter.Scan(&raw) {
		msg, decErr := chat.UnmarshalMessage([]byte(raw))
		if decErr != nil {
			err = fmt.Errorf("cassandra.Store.Read: decode message: %w", decErr)
			return nil, err
		}
		out = append(out, msg)
	}
	if err = iter.Close(); err != nil {
		return nil, fmt.Errorf("cassandra.Store.Read: %w", err)
	}
	return out, nil
}

// Clear drops every row for conversationID. Unknown ids are a no-op.
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, "cassandra", conversationID)
	defer func() { tracing.Finish(span, err) }()

	if err = s.session.Query(s.clearCQL, conversationID).WithContext(ctx).Exec(); err != nil {
		return fmt.Errorf("cassandra.Store.Clear: %w", err)
	}
	return nil
}
