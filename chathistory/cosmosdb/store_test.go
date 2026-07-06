package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chathistory/cosmosdb"
	"github.com/Tangerg/lynx/core/model/chat/history"
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
	var _ history.Store = (*cosmosdb.Store)(nil)
}
