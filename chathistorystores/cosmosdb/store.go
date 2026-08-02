package cosmosdb

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/data/azcosmos"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/core/chat"
	"github.com/Tangerg/lynx/internal/chathistorykit/codec"
	"github.com/Tangerg/lynx/internal/chathistorykit/sequence"
	"github.com/Tangerg/lynx/internal/chathistoryotel"
)

// Config configures [New]. Only [Config.Container] is required.
type Config struct {
	// Container is the live Cosmos container handle. Required. The
	// container's partition key MUST be `/conversation_id`.
	Container *azcosmos.ContainerClient
}

var (
	_ chathistory.Store  = (*Store)(nil)
	_ chathistory.Lister = (*Store)(nil)
)

// Store is a Cosmos DB-backed [chathistory.Store]. Construct via [New].
type Store struct {
	container *azcosmos.ContainerClient
	sequence  sequence.Generator
}

// New builds a [Store] from cfg.
func New(cfg Config) (*Store, error) {
	if cfg.Container == nil {
		return nil, errors.New("cosmosdb: container is required")
	}
	return &Store{container: cfg.Container}, nil
}

// document is the wire shape stored in Cosmos. The struct tags match
// the JSON the SDK expects.
type document struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversation_id"`
	Sequence       string `json:"seq"`
	Message        string `json:"message"`
	CreatedAt      string `json:"created_at"`
}

// Write creates one document per message. Random document IDs prevent
// concurrent writers from overwriting each other; seq preserves argument order
// within one call. Retried calls append fresh documents and are not idempotent.
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

	ctx, span := tracing.StartWrite(ctx, tracing.AzureCosmosDB, conversationID, len(messages))
	defer func() { tracing.Finish(span, err) }()

	encoded, err := codec.EncodeMessages(messages)
	if err != nil {
		return fmt.Errorf("cosmosdb: write: encode messages: %w", err)
	}
	partitionKey := azcosmos.NewPartitionKeyString(conversationID)
	now := time.Now().UTC()
	sequenceBase := s.sequence.Reserve(len(encoded))
	createdAt := now.Format(time.RFC3339Nano)

	for index, raw := range encoded {
		messageSequence := sequenceBase + int64(index)
		storedDocument := document{
			ID:             rand.Text(),
			ConversationID: conversationID,
			Sequence:       formatSequence(messageSequence),
			Message:        string(raw),
			CreatedAt:      createdAt,
		}
		body, marshalErr := json.Marshal(storedDocument)
		if marshalErr != nil {
			err = fmt.Errorf("cosmosdb: write: marshal message %d: %w", index, marshalErr)
			return err
		}
		if _, err = s.container.CreateItem(ctx, partitionKey, body, nil); err != nil {
			return fmt.Errorf("cosmosdb: write: create message %d: %w", index, err)
		}
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

	ctx, span := tracing.StartRead(ctx, tracing.AzureCosmosDB, conversationID)
	defer func() { tracing.RecordReadResult(span, err, len(storedMessages)) }()

	partitionKey := azcosmos.NewPartitionKeyString(conversationID)
	query := "SELECT c.id, c.seq, c.message FROM c WHERE c.conversation_id = @cid"
	queryOptions := &azcosmos.QueryOptions{
		QueryParameters: []azcosmos.QueryParameter{
			{Name: "@cid", Value: conversationID},
		},
	}

	documents := []document{}
	pager := s.container.NewQueryItemsPager(query, partitionKey, queryOptions)
	for pager.More() {
		response, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("cosmosdb: read: query page: %w", err)
		}
		for _, item := range response.Items {
			var projected document
			if err := json.Unmarshal(item, &projected); err != nil {
				return nil, fmt.Errorf("cosmosdb: read: decode document: %w", err)
			}
			if projected.ID == "" {
				return nil, errors.New("cosmosdb: read: document is missing ID")
			}
			if !validSequence(projected.Sequence) {
				return nil, fmt.Errorf("cosmosdb: read: document %q has invalid sequence %q", projected.ID, projected.Sequence)
			}
			documents = append(documents, projected)
		}
	}
	slices.SortFunc(documents, compareDocuments)
	storedMessages = make([]chat.Message, 0, len(documents))
	for index, document := range documents {
		message, err := codec.DecodeMessage([]byte(document.Message))
		if err != nil {
			return nil, fmt.Errorf("cosmosdb: read: decode message %d: %w", index, err)
		}
		storedMessages = append(storedMessages, message)
	}
	return storedMessages, nil
}

func formatSequence(sequence int64) string {
	return fmt.Sprintf("%019d", sequence)
}

func validSequence(sequence string) bool {
	value, err := strconv.ParseInt(sequence, 10, 64)
	return err == nil && value >= 0 && formatSequence(value) == sequence
}

func compareDocuments(left, right document) int {
	if order := cmp.Compare(left.Sequence, right.Sequence); order != 0 {
		return order
	}
	return cmp.Compare(left.ID, right.ID)
}

// Conversations gathers distinct IDs with a cross-partition query and returns
// them in lexical order.
func (s *Store) Conversations(ctx context.Context) (ids []string, err error) {
	if err = ctx.Err(); err != nil {
		return nil, err
	}

	ctx, span := tracing.StartList(ctx, tracing.AzureCosmosDB)
	defer func() { tracing.RecordListResult(span, err, len(ids)) }()

	// Empty partition key + a WHERE-less projection runs cross-partition;
	// SELECT DISTINCT VALUE is a simple projection the gateway can serve.
	query := "SELECT DISTINCT VALUE c.conversation_id FROM c"

	ids = []string{}
	pager := s.container.NewQueryItemsPager(query, azcosmos.NewPartitionKey(), nil)
	for pager.More() {
		response, pageErr := pager.NextPage(ctx)
		if pageErr != nil {
			return nil, fmt.Errorf("cosmosdb: list conversations: query page: %w", pageErr)
		}
		for _, item := range response.Items {
			var id string
			if err = json.Unmarshal(item, &id); err != nil {
				return nil, fmt.Errorf("cosmosdb: list conversations: decode ID: %w", err)
			}
			if err = chathistory.ValidateConversationID(id); err != nil {
				return nil, fmt.Errorf("cosmosdb: list conversations: invalid stored ID %q: %w", id, err)
			}
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// Clear deletes every document for conversationID. Cosmos has no
// bulk-delete for a partition, so each id is enumerated and
// deleted individually — fine for chat history sizes.
func (s *Store) Clear(ctx context.Context, conversationID string) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = chathistory.ValidateConversationID(conversationID); err != nil {
		return err
	}

	ctx, span := tracing.StartClear(ctx, tracing.AzureCosmosDB, conversationID)
	defer func() { tracing.Finish(span, err) }()

	partitionKey := azcosmos.NewPartitionKeyString(conversationID)
	query := "SELECT c.id FROM c WHERE c.conversation_id = @cid"
	queryOptions := &azcosmos.QueryOptions{
		QueryParameters: []azcosmos.QueryParameter{
			{Name: "@cid", Value: conversationID},
		},
	}

	// Deleting while paging the same query can skip items (the
	// continuation token is computed against the mutating result set),
	// so each round re-runs the query from scratch and deletes one
	// page, until the query comes back empty.
	for {
		pager := s.container.NewQueryItemsPager(query, partitionKey, queryOptions)
		if !pager.More() {
			return nil
		}
		response, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("cosmosdb: clear: query document IDs: %w", err)
		}
		if len(response.Items) == 0 {
			return nil
		}
		for _, item := range response.Items {
			var projected struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(item, &projected); err != nil {
				return fmt.Errorf("cosmosdb: clear: decode document ID: %w", err)
			}
			if _, err := s.container.DeleteItem(ctx, partitionKey, projected.ID, nil); err != nil {
				return fmt.Errorf("cosmosdb: clear: delete document %q: %w", projected.ID, err)
			}
		}
	}
}
