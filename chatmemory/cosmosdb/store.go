package cosmosdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"

	"github.com/Tangerg/lynx/chatmemory/internal/tracing"
	"github.com/Tangerg/lynx/core/model/chat"
	"github.com/Tangerg/lynx/core/model/chat/memory"
)

const Provider = "CosmosDBChatMemory"

// StoreConfig configures [NewStore]. Only [StoreConfig.Container] is
// required.
type StoreConfig struct {
	// Container is the live Cosmos container handle. Required. The
	// container's partition key MUST be `/conversation_id`.
	Container *azcosmos.ContainerClient
}

func (c StoreConfig) Validate() error {
	if c.Container == nil {
		return errors.New("cosmosdb: Container is required")
	}
	return nil
}

var _ memory.Store = (*Store)(nil)

// Store is a Cosmos DB-backed [memory.Store]. Construct via
// [NewStore].
type Store struct {
	container *azcosmos.ContainerClient
}

// NewStore builds a [Store] from cfg.
func NewStore(cfg StoreConfig) (*Store, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Store{container: cfg.Container}, nil
}

// document is the wire shape stored in Cosmos. The struct tags match
// the JSON the SDK expects.
type document struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Seq            int64  `json:"seq"`
	Message        string `json:"message"`
	CreatedAt      string `json:"created_at"`
}

// Write upserts every message under conversationID. The synthesized
// id (`<conversation_id>_<seq>`) is monotone so re-runs of a Write
// with the same batch are idempotent at the document level.
func (s *Store) Write(ctx context.Context, conversationID string, messages ...chat.Message) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if len(messages) == 0 {
		return nil
	}

	ctx, span := tracing.StartWrite(ctx, "cosmosdb", conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	pk := azcosmos.NewPartitionKeyString(conversationID)
	now := time.Now().UTC()
	seqBase := now.UnixNano()
	createdAt := now.Format(time.RFC3339Nano)

	for i, msg := range messages {
		raw, encErr := encodeMessage(msg)
		if encErr != nil {
			err = fmt.Errorf("cosmosdb.Store.Write: encode message: %w", encErr)
			return err
		}
		seq := seqBase + int64(i)
		doc := document{
			ID:             conversationID + "_" + strconv.FormatInt(seq, 10),
			ConversationID: conversationID,
			Seq:            seq,
			Message:        string(raw),
			CreatedAt:      createdAt,
		}
		body, marshalErr := json.Marshal(doc)
		if marshalErr != nil {
			err = fmt.Errorf("cosmosdb.Store.Write: marshal doc: %w", marshalErr)
			return err
		}
		if _, err = s.container.UpsertItem(ctx, pk, body, nil); err != nil {
			return fmt.Errorf("cosmosdb.Store.Write: upsert: %w", err)
		}
	}
	return nil
}

// Read returns every message stored under conversationID in
// insertion order.
func (s *Store) Read(ctx context.Context, conversationID string) (out []chat.Message, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartRead(ctx, "cosmosdb", conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(out)) }()

	pk := azcosmos.NewPartitionKeyString(conversationID)
	query := "SELECT c.message FROM c WHERE c.conversation_id = @cid ORDER BY c.seq ASC"
	opts := &azcosmos.QueryOptions{
		QueryParameters: []azcosmos.QueryParameter{
			{Name: "@cid", Value: conversationID},
		},
	}

	out = []chat.Message{}
	pager := s.container.NewQueryItemsPager(query, pk, opts)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("cosmosdb.Store.Read: %w", err)
		}
		for _, item := range resp.Items {
			var projected struct {
				Message string `json:"message"`
			}
			if err := json.Unmarshal(item, &projected); err != nil {
				return nil, fmt.Errorf("cosmosdb.Store.Read: unmarshal item: %w", err)
			}
			msg, err := chat.UnmarshalMessage([]byte(projected.Message))
			if err != nil {
				return nil, fmt.Errorf("cosmosdb.Store.Read: decode message: %w", err)
			}
			out = append(out, msg)
		}
	}
	return out, nil
}

// Clear deletes every document for conversationID. Cosmos has no
// bulk-delete for a partition, so we enumerate ids and issue
// individual DeleteItem calls — fine for chat-memory sizes.
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, "cosmosdb", conversationID)
	defer func() { tracing.Finish(span, err) }()

	pk := azcosmos.NewPartitionKeyString(conversationID)
	query := "SELECT c.id FROM c WHERE c.conversation_id = @cid"
	opts := &azcosmos.QueryOptions{
		QueryParameters: []azcosmos.QueryParameter{
			{Name: "@cid", Value: conversationID},
		},
	}

	pager := s.container.NewQueryItemsPager(query, pk, opts)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("cosmosdb.Store.Clear: %w", err)
		}
		for _, item := range resp.Items {
			var projected struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(item, &projected); err != nil {
				return fmt.Errorf("cosmosdb.Store.Clear: unmarshal id: %w", err)
			}
			if _, err := s.container.DeleteItem(ctx, pk, projected.ID, nil); err != nil {
				return fmt.Errorf("cosmosdb.Store.Clear: delete %q: %w", projected.ID, err)
			}
		}
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
