package cassandra

import (
	"context"
	"testing"

	"github.com/gocql/gocql"

	"github.com/Tangerg/scope/core/document"
	"github.com/Tangerg/scope/core/embedding"
)

type ownershipBatcher struct{}

func (ownershipBatcher) Batch(_ context.Context, documents []*document.Document) ([][]*document.Document, error) {
	return [][]*document.Document{documents}, nil
}

func TestNewStoreOwnsMetadataColumns(t *testing.T) {
	columns := []MetadataColumn{{Name: "tenant", CQLType: "text"}}
	store, err := NewStore(t.Context(), StoreConfig{
		Session:         &gocql.Session{},
		MetadataColumns: columns,
		EmbeddingModel: embedding.ModelFunc(func(context.Context, *embedding.Request) (*embedding.Response, error) {
			return nil, nil
		}),
		DocumentBatcher: ownershipBatcher{},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	columns[0].Name = "mutated"
	if got := store.metadataColumns[0].Name; got != "tenant" {
		t.Fatalf("store retained caller-owned MetadataColumns: got %q", got)
	}
}
