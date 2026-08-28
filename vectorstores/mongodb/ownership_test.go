package mongodb

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/embedding"
)

type ownershipBatcher struct{}

func (ownershipBatcher) Batch(_ context.Context, documents []*document.Document) ([][]*document.Document, error) {
	return [][]*document.Document{documents}, nil
}

func TestNewStoreOwnsMetadataFields(t *testing.T) {
	client, err := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })

	fields := []string{"tenant"}
	store, err := NewStore(t.Context(), StoreConfig{
		Collection:             client.Database("scope").Collection("documents"),
		MetadataFieldsToFilter: fields,
		EmbeddingModel: embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
			return nil, nil
		}),
		DocumentBatcher: ownershipBatcher{},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	fields[0] = "mutated"
	if got := store.metadataFieldsToFilter[0]; got != "tenant" {
		t.Fatalf("store retained caller-owned MetadataFieldsToFilter: got %q", got)
	}
}
