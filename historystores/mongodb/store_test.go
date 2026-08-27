package mongodb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/scope/historystores/mongodb"
)

func TestNewStoreRequiresCollection(t *testing.T) {
	config := mongodb.StoreConfig{}
	if err := config.Validate(); err == nil {
		t.Fatal("StoreConfig.Validate should reject a nil Collection")
	}
	_, err := mongodb.NewStore(t.Context(), config)
	if err == nil {
		t.Fatal("expected error when Collection is nil")
	}
	if !strings.Contains(err.Error(), "collection") {
		t.Fatalf("err = %v; should mention collection", err)
	}
}
