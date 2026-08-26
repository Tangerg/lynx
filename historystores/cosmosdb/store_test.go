package cosmosdb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/historystores/cosmosdb"
)

func TestNewRequiresContainer(t *testing.T) {
	cfg := cosmosdb.Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Config.Validate should reject a nil Container")
	}
	_, err := cosmosdb.New(cfg)
	if err == nil {
		t.Fatal("expected error when Container is nil")
	}
	if !strings.Contains(err.Error(), "container") {
		t.Fatalf("err = %v; should mention container", err)
	}
}
