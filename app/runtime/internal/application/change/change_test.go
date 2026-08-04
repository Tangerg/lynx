package change

import "testing"

func TestResourcesReturnsACallerOwnedCatalog(t *testing.T) {
	resources := Resources()
	resources[0] = 0
	if Resources()[0] != Sessions {
		t.Fatal("mutating one Resources result rewrote the change vocabulary")
	}
}
