package mongodb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chatmemory/mongodb"
	chathistory "github.com/Tangerg/lynx/core/model/chat/history"
)

func TestStoreConfig_CollectionRequired(t *testing.T) {
	_, err := mongodb.NewStore(mongodb.StoreConfig{})
	if err == nil {
		t.Fatal("expected error when Collection is nil")
	}
	if !strings.Contains(err.Error(), "Collection") {
		t.Fatalf("err = %v; should mention Collection", err)
	}
}

func TestStoreConfig_NilConfig(t *testing.T) {
	if _, err := mongodb.NewStore(mongodb.StoreConfig{}); err == nil {
		t.Fatal("expected error when config is nil")
	}
}

func TestStore_ImplementsHistoryStore(t *testing.T) {
	var _ chathistory.Store = (*mongodb.Store)(nil)
}
