package mongodb_test

import (
	"strings"
	"testing"

	"github.com/Tangerg/lynx/chathistorystores/mongodb"
)

func TestNewRequiresCollection(t *testing.T) {
	_, err := mongodb.New(t.Context(), mongodb.Config{})
	if err == nil {
		t.Fatal("expected error when Collection is nil")
	}
	if !strings.Contains(err.Error(), "collection") {
		t.Fatalf("err = %v; should mention collection", err)
	}
}
