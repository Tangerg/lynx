package postgres

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Tangerg/lynx/chatmemory/internal/tracing"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

// Provider names the backend for observability layers that branch
// on store identity.
const Provider = "PostgresChatMemory"

// Default identifiers used when [StoreConfig] leaves them blank.
const (
	DefaultSchemaName  = "public"
	DefaultTableName   = "chat_memory"
	DefaultIndexSuffix = "_conversation_idx"
)

// identPattern matches the standard SQL unquoted-identifier shape — a
// leading letter or underscore followed by letters / digits /
// underscores. Schema names, table names and index names are all
// interpolated into DDL/queries, so we reject anything that doesn't
// match before issuing SQL.
var identPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// StoreConfig configures [NewStore]. Only [StoreConfig.Pool] is
// required; the rest fall back to documented defaults.
type StoreConfig struct {
	// Context is used for the initial schema bootstrap when
	// InitializeSchema is true. Optional: defaults to
	// context.Background().
	Context context.Context

	// Pool is the pgx connection pool. Required. The store does not
	// take ownership — callers close the pool themselves.
	Pool *pgxpool.Pool

	// SchemaName is the PostgreSQL schema that holds the chat-memory
	// table. Optional: defaults to [DefaultSchemaName] ("public").
	SchemaName string

	// TableName is the table that stores serialized messages.
	// Optional: defaults to [DefaultTableName] ("chat_memory").
	TableName string

	// IndexName overrides the conversation-id index name generated
	// when InitializeSchema is true. Optional: defaults to
	// "<TableName><DefaultIndexSuffix>".
	IndexName string

	// InitializeSchema, when true, creates the table and index if
	// they don't already exist. When false the store assumes the
	// schema is already provisioned.
	InitializeSchema bool
}

func (c StoreConfig) Validate() error {
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Pool == nil {
		return errors.New("postgres: Pool is required")
	}

	c.SchemaName = cmpOr(c.SchemaName, DefaultSchemaName)
	c.TableName = cmpOr(c.TableName, DefaultTableName)
	c.IndexName = cmpOr(c.IndexName, c.TableName+DefaultIndexSuffix)

	idents := map[string]string{
		"SchemaName": c.SchemaName,
		"TableName":  c.TableName,
		"IndexName":  c.IndexName,
	}
	for name, value := range idents {
		if !identPattern.MatchString(value) {
			return fmt.Errorf("postgres: %s=%q must match %s",
				name, value, identPattern)
		}
	}
	return nil
}

// cmpOr is a local stand-in for cmp.Or — first non-zero string wins.
func cmpOr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

var _ memory.Store = (*Store)(nil)

// Store is a PostgreSQL-backed [memory.Store]. Construct via
// [NewStore].
//
// Schema (created when [StoreConfig.InitializeSchema] is true):
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
	pool *pgxpool.Pool

	// Pre-formatted SQL — interpolated identifiers are validated at
	// construction time so the hot path is plain parameter binding.
	readSQL   string
	writeSQL  string
	clearSQL  string
	createSQL []string
}

// NewStore builds a [Store] from cfg. When
// [StoreConfig.InitializeSchema] is true the table and index are
// created if they don't already exist using [StoreConfig.Context].
func NewStore(cfg StoreConfig) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

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
		if err := s.initSchema(cfg.Context); err != nil {
			return nil, fmt.Errorf("postgres: initialize schema: %w", err)
		}
	}

	return s, nil
}

// initSchema creates the table + index if they don't exist. Idempotent.
func (s *Store) initSchema(ctx context.Context) error {
	for _, stmt := range s.createSQL {
		if _, err := s.pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// Write appends every message under conversationID. Messages within a
// batch are inserted in order via [pgx.Batch] so ordering is
// guaranteed even under concurrent writers on the same conversation.
// No-op when messages is empty.
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	ctx, span := tracing.StartWrite(ctx, "postgres", conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	batch := &pgx.Batch{}
	for _, msg := range messages {
		raw, err := encodeMessage(msg)
		if err != nil {
			return fmt.Errorf("postgres.Store.Write: encode message: %w", err)
		}
		batch.Queue(s.writeSQL, conversationID, raw)
	}

	br := s.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range messages {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("postgres.Store.Write: %w", err)
		}
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order. An empty slice is returned for unknown ids.
func (s *Store) Read(ctx context.Context, conversationID string) (out []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartRead(ctx, "postgres", conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(out)) }()

	rows, err := s.pool.Query(ctx, s.readSQL, conversationID)
	if err != nil {
		return nil, fmt.Errorf("postgres.Store.Read: %w", err)
	}
	defer rows.Close()

	out = []chat.Message{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("postgres.Store.Read: scan: %w", err)
		}
		msg, err := chat.UnmarshalMessage(raw)
		if err != nil {
			return nil, fmt.Errorf("postgres.Store.Read: decode message: %w", err)
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("postgres.Store.Read: %w", err)
	}
	return out, nil
}

// Clear drops every message stored under conversationID. Unknown ids
// are silently ignored.
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, "postgres", conversationID)
	defer func() { tracing.Finish(span, err) }()

	if _, err = s.pool.Exec(ctx, s.clearSQL, conversationID); err != nil {
		return fmt.Errorf("postgres.Store.Clear: %w", err)
	}
	return nil
}

// encodeMessage marshals msg to JSON via the message type's
// MarshalJSON. nil-message safe (returns an error rather than
// inserting "null").
func encodeMessage(msg chat.Message) ([]byte, error) {
	if msg == nil {
		return nil, errors.New("message must not be nil")
	}
	// Each concrete chat.Message type implements MarshalJSON to emit
	// the canonical MessageParams wire shape with a Type discriminator;
	// chat.UnmarshalMessage decodes the same shape back.
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
