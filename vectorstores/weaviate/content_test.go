package weaviate

import (
	"testing"

	"github.com/Tangerg/scope/core/document"
)

func TestBuildObjectsAlwaysStoresDocumentText(t *testing.T) {
	t.Parallel()

	store := &Store{className: "Knowledge"}
	objects, err := store.buildObjects(
		[]*document.Document{{ID: "12345678-1234-1234-1234-123456789012", Text: "content"}},
		[][]float64{{1, 0}},
	)
	if err != nil {
		t.Fatal(err)
	}
	properties, ok := objects[0].Properties.(map[string]any)
	if !ok {
		t.Fatalf("properties type = %T, want map[string]any", objects[0].Properties)
	}
	if got := properties[fieldContent]; got != "content" {
		t.Fatalf("stored content = %v, want content", got)
	}
}
