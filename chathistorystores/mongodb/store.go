package mongodb

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/internal/chathistorykit/codec"
	"github.com/Tangerg/lynx/internal/chathistorykit/sequence"
	"github.com/Tangerg/lynx/internal/chathistoryotel"
)

const (
	fieldID             = "_id"
	fieldConversationID = "conversation_id"
	fieldSequence       = "seq"
	fieldMessage        = "message"
	fieldCreatedAt      = "created_at"
)

// Config configures [New]. Only [Config.Collection] is required.
type Config struct {
	// Collection is the live MongoDB collection. Required. The store
	// does not take ownership of the underlying client.
	Collection *mongo.Collection

	// InitializeSchema, when true, ensures an index on
	// (conversation_id, seq, _id) exists. Idempotent.
	InitializeSchema bool
}

var (
	_ chathistory.Store  = (*Store)(nil)
	_ chathistory.Lister = (*Store)(nil)
)

// Store is a MongoDB-backed [chathistory.Store]. Construct via [New].
type Store struct {
	collection *mongo.Collection
	sequence   sequence.Generator
}

// New builds a [Store] from cfg. ctx bounds optional index initialization.
func New(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.Collection == nil {
		return nil, errors.New("mongodb: collection is required")
	}
	s := &Store{collection: cfg.Collection}
	if cfg.InitializeSchema {
		if err := s.initIndex(ctx); err != nil {
			return nil, fmt.Errorf("mongodb: initialize schema: %w", err)
		}
	}
	return s, nil
}

// initIndex creates an ascending compound index on (conversation_id, seq, _id)
// so per-conversation reads sort efficiently and ties are deterministic.
func (s *Store) initIndex(ctx context.Context) error {
	_, err := s.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: fieldConversationID, Value: 1},
			{Key: fieldSequence, Value: 1},
			{Key: fieldID, Value: 1},
		},
		Options: options.Index().SetName("conversation_id_sequence_idx"),
	})
	return err
}

// Write inserts every message under conversationID via InsertMany. A reserved
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

	ctx, span := tracing.StartWrite(ctx, tracing.MongoDB, conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	encoded, err := codec.EncodeMessages(messages)
	if err != nil {
		return fmt.Errorf("mongodb: write: encode messages: %w", err)
	}
	now := time.Now().UTC()
	sequenceBase := s.sequence.Reserve(len(encoded))
	docs := make([]any, 0, len(encoded))
	for index, raw := range encoded {
		docs = append(docs, bson.M{
			fieldConversationID: conversationID,
			fieldSequence:       sequenceBase + int64(index),
			fieldMessage:        string(raw),
			fieldCreatedAt:      now,
		})
	}

	if _, err = s.collection.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("mongodb: write: insert messages: %w", err)
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order.
func (s *Store) Read(ctx context.Context, conversationID string) (storedMessages []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartRead(ctx, tracing.MongoDB, conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(storedMessages)) }()

	var cursor *mongo.Cursor
	cursor, err = s.collection.Find(ctx,
		bson.M{fieldConversationID: conversationID},
		options.Find().SetSort(bson.D{
			{Key: fieldSequence, Value: 1},
			{Key: fieldID, Value: 1},
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("mongodb: read: find messages: %w", err)
	}

	var documents []struct {
		ID       bson.ObjectID `bson:"_id"`
		Sequence int64         `bson:"seq"`
		Message  string        `bson:"message"`
	}
	if err = cursor.All(ctx, &documents); err != nil {
		return nil, fmt.Errorf("mongodb: read: decode documents: %w", err)
	}
	storedMessages = make([]chat.Message, 0, len(documents))
	for index, document := range documents {
		if document.ID.IsZero() {
			return nil, fmt.Errorf("mongodb: read: document %d is missing ID", index)
		}
		if document.Sequence <= 0 {
			return nil, fmt.Errorf("mongodb: read: document %s has invalid sequence %d", document.ID.Hex(), document.Sequence)
		}
		message, err := codec.DecodeMessage([]byte(document.Message))
		if err != nil {
			return nil, fmt.Errorf("mongodb: read: decode message %d: %w", index, err)
		}
		storedMessages = append(storedMessages, message)
	}
	return storedMessages, nil
}

// Clear drops every document for conversationID. Unknown ids result
// in a no-op (DeleteMany matches zero docs).
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, tracing.MongoDB, conversationID)
	defer func() { tracing.Finish(span, err) }()

	if _, err = s.collection.DeleteMany(ctx, bson.M{fieldConversationID: conversationID}); err != nil {
		return fmt.Errorf("mongodb: clear: delete messages: %w", err)
	}
	return nil
}

// Conversations returns distinct conversation IDs in lexical order. It is a
// deliberate cross-conversation scan for operational tasks.
func (s *Store) Conversations(ctx context.Context) (ids []string, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartList(ctx, tracing.MongoDB)
	defer func() { tracing.RecordListResult(span, err, len(ids)) }()

	ids = []string{}
	if err = s.collection.Distinct(ctx, fieldConversationID, bson.D{}).Decode(&ids); err != nil {
		return nil, fmt.Errorf("mongodb: list conversations: query distinct IDs: %w", err)
	}
	for _, id := range ids {
		if err := chathistory.ValidateConversationID(id); err != nil {
			return nil, fmt.Errorf("mongodb: list conversations: invalid stored ID %q: %w", id, err)
		}
	}
	slices.Sort(ids)
	return ids, nil
}
