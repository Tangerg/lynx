package mongodb

import (
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestStoreConfigRejectsStorageFieldCollisions(t *testing.T) {
	config := StoreConfig{ContentField: "payload", EmbeddingPath: "payload"}
	config.applyDefaults()
	if err := config.validateFieldLayout(); err == nil {
		t.Fatal("validateFieldLayout accepted colliding content and embedding fields")
	}
}

func TestStoreConfigRejectsDuplicateMetadataFilterFields(t *testing.T) {
	config := StoreConfig{MetadataFieldsToFilter: []string{"tenant", "tenant"}}
	if err := config.validateMetadataFields(); err == nil {
		t.Fatal("validateMetadataFields accepted a duplicate field")
	}
}

func TestStoreConfigAlwaysOwnsMetadataSubdocument(t *testing.T) {
	config := StoreConfig{}
	config.applyDefaults()
	if config.MetadataField != DefaultMetadataField {
		t.Fatalf("MetadataField = %q", config.MetadataField)
	}
}

func TestToMatchDecodesOrderedMetadataDocument(t *testing.T) {
	store := &Store{contentField: "content", embeddingPath: "embedding", metadataField: "metadata"}
	result, err := store.toMatch(bson.M{
		defaultIDField: "doc-1",
		"content":      "hello",
		scoreField:     0.75,
		"metadata":     bson.D{{Key: "tenant", Value: "acme"}},
	})
	if err != nil {
		t.Fatalf("toMatch: %v", err)
	}
	values, err := result.Document.Metadata.Values()
	if err != nil {
		t.Fatalf("metadata Values: %v", err)
	}
	if got := values["tenant"]; got != "acme" {
		t.Fatalf("tenant = %#v", got)
	}
}

func TestToMatchRejectsMalformedMetadataDocument(t *testing.T) {
	store := &Store{contentField: "content", embeddingPath: "embedding", metadataField: "metadata"}
	_, err := store.toMatch(bson.M{
		defaultIDField: "doc-1",
		"content":      "hello",
		scoreField:     0.75,
		"metadata":     "not-a-document",
	})
	if err == nil || !strings.Contains(err.Error(), `field "metadata" must be a document`) {
		t.Fatalf("toMatch error = %v", err)
	}
}
