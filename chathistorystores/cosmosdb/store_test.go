package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chathistorystores/cosmosdb"
)

func TestNewRequiresContainer(t *testing.T) {
	_, err := cosmosdb.New(cosmosdb.Config{})
	if err == nil {
		t.Fatal("expected error when Container is nil")
	}
	if !strings.Contains(err.Error(), "container") {
		t.Fatalf("err = %v; should mention container", err)
	}
}
