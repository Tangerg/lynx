package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/cosmosdb"
)

func TestNewRequiresContainer(t *testing.T) {
	_, err := cosmosdb.New(cosmosdb.Config{})
	if err == nil {
		t.Fatal("expected error when Container is nil")
	}
	if !strings.Contains(err.Error(), "Container") {
		t.Fatalf("err = %v; should mention Container", err)
	}
}

func TestStoreImplementsHistoryStore(t *testing.T) {
	var _ chathistory.Store = (*cosmosdb.Store)(nil)
}
