package mongodb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chathistory"
	"github.com/Tangerg/lynx/chathistory/mongodb"
)

func TestNewRequiresCollection(t *testing.T) {
	_, err := mongodb.New(mongodb.Config{})
	if err == nil {
		t.Fatal("expected error when Collection is nil")
	}
	if !strings.Contains(err.Error(), "Collection") {
		t.Fatalf("err = %v; should mention Collection", err)
	}
}

func TestStoreImplementsHistoryStore(t *testing.T) {
	var _ chathistory.Store = (*mongodb.Store)(nil)
}
