package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/scope/historystores/cosmosdb"
)

func TestNewStoreRequiresContainer(t *testing.T) {
	config := cosmosdb.StoreConfig{}
	if err := config.Validate(); err == nil {
		t.Fatal("StoreConfig.Validate should reject a nil Container")
	}
	_, err := cosmosdb.NewStore(config)
	if err == nil {
		t.Fatal("expected error when Container is nil")
	}
	if !strings.Contains(err.Error(), "container") {
		t.Fatalf("err = %v; should mention container", err)
	}
}
