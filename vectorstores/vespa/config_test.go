package vespa

import "testing"

func TestStoreConfigRejectsAnAbsentEndpoint(t *testing.T) {
	if err := (StoreConfig{}).Validate(); err == nil {
		t.Fatal("Validate accepted an absent endpoint")
	}
}
