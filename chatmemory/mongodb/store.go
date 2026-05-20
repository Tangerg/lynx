// Package mongodb is a [memory.Store] backed by MongoDB via the
// official mongo-driver v2.
//
// Each message is a document in the configured collection:
//
//	{
//	    "_id":             ObjectId(...),     // assigned by the driver
//	    "conversation_id": "u-42",
//	    "message":         "<json>",          // canonical chat.Message wire shape
//	    "created_at":      ISODate(...),
//	}
//
// Documents are read back in `_id` ascending order — ObjectIDs are
// monotonically increasing within a single InsertMany batch, so
// chronological order is recovered without a separate `seq` field.
//
// Example:
//
//	col := client.Database("lynx").Collection("chat_memory")
//	store, _ := mongodb.NewStore(&mongodb.StoreConfig{
//	    Collection:       col,
//	    InitializeSchema: true, // create the conversation_id index
//	})
package mongodb

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

const Provider = "MongoDBChatMemory"

const (
	fieldID             = "_id"
	fieldConversationID = "conversation_id"
	fieldMessage        = "message"
	fieldCreatedAt      = "created_at"
)

// StoreConfig configures [NewStore]. Only [StoreConfig.Collection] is
// required.
type StoreConfig struct {
	// Context is used for the schema bootstrap (index creation) when
	// InitializeSchema is true. Optional: defaults to
	// context.Background().
	Context context.Context

	// Collection is the live MongoDB collection. Required. The store
	// does not take ownership of the underlying client.
	Collection *mongo.Collection

	// InitializeSchema, when true, ensures an index on
	// (conversation_id, _id) exists. Idempotent.
	InitializeSchema bool
}

func (c *StoreConfig) validate() error {
	if c == nil {
		return errors.New("mongodb: config must not be nil")
	}
	if c.Context == nil {
		c.Context = context.Background()
	}
	if c.Collection == nil {
		return errors.New("mongodb: Collection is required")
	}
	return nil
}

var _ memory.Store = (*Store)(nil)

// Store is a MongoDB-backed [memory.Store]. Construct via [NewStore].
type Store struct {
	collection *mongo.Collection
}

// NewStore builds a [Store] from cfg.
func NewStore(cfg *StoreConfig) (*Store, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	s := &Store{collection: cfg.Collection}
	if cfg.InitializeSchema {
		if err := s.initIndex(cfg.Context); err != nil {
			return nil, fmt.Errorf("mongodb: initialize schema: %w", err)
		}
	}
	return s, nil
}

// initIndex creates an ascending compound index on (conversation_id,
// _id) so per-conversation reads sort efficiently. Idempotent.
func (s *Store) initIndex(ctx context.Context) error {
	_, err := s.collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: fieldConversationID, Value: 1},
			{Key: fieldID, Value: 1},
		},
		Options: options.Index().SetName("conversation_id_seq_idx"),
	})
	return err
}

// Write inserts every message under conversationID via InsertMany.
// ObjectIDs are assigned at the driver — strictly increasing within
// a batch, so chronological order is preserved on Read.
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	now := time.Now().UTC()
	docs := make([]any, 0, len(messages))
	for _, msg := range messages {
		raw, err := encodeMessage(msg)
		if err != nil {
			return fmt.Errorf("mongodb.Store.Write: encode message: %w", err)
		}
		docs = append(docs, bson.M{
			fieldConversationID: conversationID,
			fieldMessage:        string(raw),
			fieldCreatedAt:      now,
		})
	}

	if _, err := s.collection.InsertMany(ctx, docs); err != nil {
		return fmt.Errorf("mongodb.Store.Write: %w", err)
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order.
func (s *Store) Read(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	cursor, err := s.collection.Find(ctx,
		bson.M{fieldConversationID: conversationID},
		options.Find().SetSort(bson.D{{Key: fieldID, Value: 1}}),
	)
	if err != nil {
		return nil, fmt.Errorf("mongodb.Store.Read: %w", err)
	}
	defer cursor.Close(ctx)

	out := []chat.Message{}
	for cursor.Next(ctx) {
		var doc struct {
			Message string `bson:"message"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("mongodb.Store.Read: decode doc: %w", err)
		}
		msg, err := chat.UnmarshalMessage([]byte(doc.Message))
		if err != nil {
			return nil, fmt.Errorf("mongodb.Store.Read: decode message: %w", err)
		}
		out = append(out, msg)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("mongodb.Store.Read: cursor: %w", err)
	}
	return out, nil
}

// Clear drops every document for conversationID. Unknown ids result
// in a no-op (DeleteMany matches zero docs).
func (s *Store) Clear(ctx context.Context, conversationID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := s.collection.DeleteMany(ctx, bson.M{fieldConversationID: conversationID}); err != nil {
		return fmt.Errorf("mongodb.Store.Clear: %w", err)
	}
	return nil
}

// encodeMessage marshals msg via its MarshalJSON.
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
