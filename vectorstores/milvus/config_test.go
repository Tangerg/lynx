package milvus

import "testing"

func TestStoreConfigRejectsAnAbsentClient(t *testing.T) {
	if err := (StoreConfig{}).Validate(); err == nil {
		t.Fatal("Validate accepted an absent client")
	}
}
