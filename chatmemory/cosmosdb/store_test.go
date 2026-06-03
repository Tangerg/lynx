package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatmemory/cosmosdb"
	chatmem "github.com/Tangerg/lynx/core/model/chat/memory"
)

func TestStoreConfig_ContainerRequired(t *testing.T) {
	_, err := cosmosdb.NewStore(cosmosdb.StoreConfig{})
	if err == nil {
		t.Fatal("expected error when Container is nil")
	}
	if !strings.Contains(err.Error(), "Container") {
		t.Fatalf("err = %v; should mention Container", err)
	}
}

func TestStoreConfig_NilConfig(t *testing.T) {
	if _, err := cosmosdb.NewStore(cosmosdb.StoreConfig{}); err == nil {
		t.Fatal("expected error when config is nil")
	}
}

func TestStore_ImplementsMemoryStore(t *testing.T) {
	var _ chatmem.Store = (*cosmosdb.Store)(nil)
}
