package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatmemory/cosmosdb"
	chathistory "github.com/Tangerg/lynx/core/model/chat/history"
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

func TestStore_ImplementsHistoryStore(t *testing.T) {
	var _ chathistory.Store = (*cosmosdb.Store)(nil)
}
