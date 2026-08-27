package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/scope/historystores/cosmosdb"
)

func TestNewStoreRequiresContainer(t *testing.T) {
	cfg := cosmosdb.StoreConfig{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("StoreConfig.Validate should reject a nil Container")
	}
	_, err := cosmosdb.NewStore(cfg)
	if err == nil {
		t.Fatal("expected error when Container is nil")
	}
	if !strings.Contains(err.Error(), "container") {
		t.Fatalf("err = %v; should mention container", err)
	}
}
