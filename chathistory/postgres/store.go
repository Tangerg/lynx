package postgres

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tangerg/lynx/chathistory"
	historystorage "github.com/Tangerg/lynx/chathistory/internal/storage"
	"github.com/Tangerg/lynx/core/chat"
)

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func validIdentifier(value string) bool {
	return identifierPattern.MatchString(value)
}

// Default identifiers used when [Config] leaves them blank.
const (
	DefaultSchemaName      = "public"
	DefaultTableName       = "chat_history"
	DefaultIndexNameSuffix = "_conversation_idx"
)

// Config configures [New]. Only [Config.Pool] is required; the rest fall back
// to documented defaults.
type Config struct {
	// Pool is the pgx connection pool. Required. The store does not
	// take ownership — callers close the pool themselves.
	Pool *pgxpool.Pool

	// SchemaName is the PostgreSQL schema that holds the chat history
	// table. Optional: defaults to [DefaultSchemaName] ("public").
	SchemaName string

	// TableName is the table that stores serialized messages.
	// Optional: defaults to [DefaultTableName] ("chat_history").
	TableName string

	// IndexName overrides the conversation-id index name generated
	// when InitializeSchema is true. Optional: defaults to
	// "<TableName><DefaultIndexNameSuffix>".
	IndexName string

	// InitializeSchema, when true, creates the table and index if
	// they don't already exist. When false the store assumes the
	// schema is already provisioned.
	InitializeSchema bool
}

// Validate reports whether c can be used to construct a [Store]. Blank
// optional identifiers are valid and are resolved to their documented
// defaults by [New].
func (c Config) Validate() error {
	if c.Pool == nil {
		return errors.New("postgres: pool is required")
	}
	for _, identifier := range [...]struct {
		name  string
		value string
	}{
		{name: "schema name", value: c.SchemaName},
		{name: "table name", value: c.TableName},
		{name: "index name", value: c.IndexName},
	} {
		if identifier.value != "" && !validIdentifier(identifier.value) {
			return fmt.Errorf("postgres: %s %q must be a valid unquoted identifier", identifier.name, identifier.value)
		}
	}
	return nil
}

var (
	_ chathistory.Store  = (*Store)(nil)
	_ chathistory.Lister = (*Store)(nil)
)

// Store is a PostgreSQL-backed [chathistory.Store]. Construct via [New].
//
// Schema (created when [Config.InitializeSchema] is true):
//
//	CREATE TABLE <schema>.<table> (
//	    seq             BIGSERIAL    PRIMARY KEY,
//	    conversation_id TEXT         NOT NULL,
//	    message         JSONB        NOT NULL,
//	    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
//	);
//	CREATE INDEX <index>
//	    ON <schema>.<table> (conversation_id, seq);
//
// `seq` is global (BIGSERIAL) so concurrent writers in different
// conversations don't contend on a per-conversation counter; ordering
// inside a single conversation is recovered by ORDER BY seq.
type Store struct {
	pool  *pgxpool.Pool
	codec historystorage.MessageCodec

	// Pre-formatted SQL — interpolated identifiers are validated at
	// construction time so the hot path is plain parameter binding.
	readSQL   string
	writeSQL  string
	clearSQL  string
	listSQL   string
	createSQL []string
}

// New builds a [Store] from cfg. ctx bounds optional schema initialization.
func New(ctx context.Context, cfg Config) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg.SchemaName = cmp.Or(cfg.SchemaName, DefaultSchemaName)
	cfg.TableName = cmp.Or(cfg.TableName, DefaultTableName)
	cfg.IndexName = cmp.Or(cfg.IndexName, cfg.TableName+DefaultIndexNameSuffix)

	qualified := cfg.SchemaName + "." + cfg.TableName
	s := &Store{
		pool: cfg.Pool,
		readSQL: fmt.Sprintf(
			"SELECT message FROM %s WHERE conversation_id = $1 ORDER BY seq",
			qualified,
		),
		writeSQL: fmt.Sprintf(
			"INSERT INTO %s (conversation_id, message) VALUES ($1, $2)",
			qualified,
		),
		clearSQL: fmt.Sprintf(
			"DELETE FROM %s WHERE conversation_id = $1",
			qualified,
		),
		listSQL: fmt.Sprintf(
			"SELECT DISTINCT conversation_id FROM %s",
			qualified,
		),
		createSQL: []string{
			fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, cfg.SchemaName),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
				seq             BIGSERIAL    PRIMARY KEY,
				conversation_id TEXT         NOT NULL,
				message         JSONB        NOT NULL,
				created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
			)`, qualified),
			fmt.Sprintf(
				`CREATE INDEX IF NOT EXISTS %s ON %s (conversation_id, seq)`,
				cfg.IndexName, qualified,
			),
		},
	}

	if cfg.InitializeSchema {
		if err := s.initSchema(ctx); err != nil {
			return nil, fmt.Errorf("postgres: initialize schema: %w", err)
		}
	}

	return s, nil
}

// initSchema creates the table + index if they don't exist. Idempotent.
func (s *Store) initSchema(ctx context.Context) error {
	for _, statement := range s.createSQL {
		if _, err := s.pool.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// Write appends every message under conversationID. Messages within one call
// are queued in order; concurrent calls may interleave. Empty writes are a
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

	encoded, err := s.codec.Encode(messages)
	if err != nil {
		return fmt.Errorf("postgres: write: encode messages: %w", err)
	}
	batch := &pgx.Batch{}
	for _, raw := range encoded {
		batch.Queue(s.writeSQL, conversationID.String(), raw)
	}

	results := s.pool.SendBatch(ctx, batch)
	if err = results.Close(); err != nil {
		return fmt.Errorf("postgres: write: execute batch: %w", err)
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

	rows, err := s.pool.Query(ctx, s.readSQL, conversationID.String())
	if err != nil {
		return nil, fmt.Errorf("postgres: read: query: %w", err)
	}
	defer rows.Close()

	storedMessages = []chat.Message{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("postgres: read: scan message: %w", err)
		}
		message, err := s.codec.Decode(raw)
		if err != nil {
			return nil, fmt.Errorf("postgres: read: decode message: %w", err)
		}
		storedMessages = append(storedMessages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: read: iterate rows: %w", err)
	}
	return storedMessages, nil
}

// Clear drops every message stored under conversationID. Unknown ids
// are silently ignored.
func (s *Store) Clear(ctx context.Context, conversationID chathistory.ConversationID) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = conversationID.Validate(); err != nil {
		return err
	}

	if _, err = s.pool.Exec(ctx, s.clearSQL, conversationID.String()); err != nil {
		return fmt.Errorf("postgres: clear: delete messages: %w", err)
	}
	return nil
}

// Conversations returns every stored conversation ID in lexical order.
func (s *Store) Conversations(ctx context.Context) (ids []chathistory.ConversationID, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	rows, err := s.pool.Query(ctx, s.listSQL)
	if err != nil {
		return nil, fmt.Errorf("postgres: list conversations: query: %w", err)
	}
	defer rows.Close()

	ids = []chathistory.ConversationID{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("postgres: list conversations: scan ID: %w", err)
		}
		conversationID := chathistory.ConversationID(id)
		if err := conversationID.Validate(); err != nil {
			return nil, fmt.Errorf("postgres: list conversations: invalid stored ID %q: %w", id, err)
		}
		ids = append(ids, conversationID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres: list conversations: iterate rows: %w", err)
	}
	slices.Sort(ids)
	return ids, nil
}
