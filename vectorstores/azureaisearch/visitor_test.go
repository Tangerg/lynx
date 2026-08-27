package azureaisearch

import (
	"testing"

	"github.com/Tangerg/scope/core/vectorstore/filter"
)

func TestAzureWildcardPattern(t *testing.T) {
	t.Parallel()

	if got, want := azureWildcardPattern(`50%_off*?\sale's`), `50*?off\*\?\\sale''s`; got != want {
		t.Fatalf("azureWildcardPattern() = %q, want %q", got, want)
	}
}

func TestCollectionMembershipUsesODataAny(t *testing.T) {
	t.Parallel()

	visitor := NewVisitor()
	if err := filter.Has("visible_to", "user-42").Accept(visitor); err != nil {
		t.Fatal(err)
	}
	if got, want := visitor.Result(), `visible_to/any(element: element eq 'user-42')`; got != want {
		t.Fatalf("Result() = %q, want %q", got, want)
	}
}
