package storetest_test

import (
	"testing"

	"github.com/Tangerg/lynx/agent/core"
	"github.com/Tangerg/lynx/agent/storetest"
)

func TestInMemoryProcessStoreContract(t *testing.T) {
	if err := storetest.TestProcessStore(t.Context(), core.NewMemoryProcessStore()); err != nil {
		t.Fatal(err)
	}
}
