package mongodb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/history/mongodb"
)

func TestNewRequiresCollection(t *testing.T) {
	cfg := mongodb.Config{}
	if err := cfg.Validate(); err == nil {
		t.Fatal("Config.Validate should reject a nil Collection")
	}
	_, err := mongodb.New(t.Context(), cfg)
	if err == nil {
		t.Fatal("expected error when Collection is nil")
	}
	if !strings.Contains(err.Error(), "collection") {
		t.Fatalf("err = %v; should mention collection", err)
	}
}
